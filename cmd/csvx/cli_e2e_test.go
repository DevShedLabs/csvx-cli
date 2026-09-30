package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// This file tests the CLI as a CLI: it builds the real binary and invokes it as a subprocess with
// real arguments against real fixture files, checking actual stdout/exit codes — not just the
// internal functions a command happens to call. See csvx-spec/AGENTS.md rule 4.7: argument-parsing
// unit tests (main_test.go) are necessary but not sufficient on their own, and every command here
// (package, extract, validate, inspect, xlsx-inspect) existed before this file with zero
// behavioral coverage.

var cliBinaryPath string

func TestMain(m *testing.M) {
	tempDir, err := os.MkdirTemp("", "csvx-cli-e2e-bin")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tempDir)
	cliBinaryPath = filepath.Join(tempDir, "csvx")
	build := exec.Command("go", "build", "-o", cliBinaryPath, ".")
	if output, err := build.CombinedOutput(); err != nil {
		panic("build csvx CLI for end-to-end tests: " + err.Error() + "\n" + string(output))
	}
	os.Exit(m.Run())
}

func runCLI(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(cliBinaryPath, args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return outBuf.String(), errBuf.String(), exitErr.ExitCode()
		}
		t.Fatalf("run csvx %v: %v", args, err)
	}
	return outBuf.String(), errBuf.String(), 0
}

func exampleXLSXPath(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "csvx-spec", "examples", "example.xlsx")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("real fixture not found at %s (expected a sibling csvx-spec checkout): %v", path, err)
	}
	return path
}

// TestCLIEndToEnd_ConvertValidateInspectExtractPackage exercises the full real-world chain a user
// would actually run: convert a real XLSX, validate it (both the CLI's own check and the canonical
// schema validator), inspect it, extract it, repackage it, and confirm the repackaged output is
// still valid and schema-conformant. Every step uses the compiled binary and real files — no
// hand-invented minimal fixtures, no calling internal functions directly.
func TestCLIEndToEnd_ConvertValidateInspectExtractPackage(t *testing.T) {
	input := exampleXLSXPath(t)
	tempDir := t.TempDir()
	convertedPath := filepath.Join(tempDir, "converted.csvx")

	if stdout, stderr, code := runCLI(t, "convert", input, convertedPath); code != 0 {
		t.Fatalf("convert failed: code=%d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if _, err := os.Stat(convertedPath); err != nil {
		t.Fatalf("convert did not produce output file: %v", err)
	}

	if stdout, stderr, code := runCLI(t, "validate", convertedPath); code != 0 {
		t.Fatalf("validate of converted output failed: code=%d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	validateAgainstSchema(t, convertedPath)

	stdout, stderr, code := runCLI(t, "inspect", convertedPath)
	if code != 0 {
		t.Fatalf("inspect failed: code=%d\nstderr=%s", code, stderr)
	}
	var inspected map[string]any
	if err := json.Unmarshal([]byte(stdout), &inspected); err != nil {
		t.Fatalf("inspect did not print valid JSON: %v\n%s", err, stdout)
	}
	if _, ok := inspected["sheets"]; !ok {
		t.Fatalf("inspect output missing \"sheets\": %s", stdout)
	}

	extractedDir := filepath.Join(tempDir, "extracted")
	if _, stderr, code := runCLI(t, "extract", convertedPath, "--output", extractedDir); code != 0 {
		t.Fatalf("extract failed: code=%d\nstderr=%s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(extractedDir, "manifest.json")); err != nil {
		t.Fatalf("extract did not produce manifest.json: %v", err)
	}

	repackagedPath := filepath.Join(tempDir, "repackaged.csvx")
	if _, stderr, code := runCLI(t, "package", extractedDir, "--output", repackagedPath); code != 0 {
		t.Fatalf("package failed: code=%d\nstderr=%s", code, stderr)
	}
	if stdout, stderr, code := runCLI(t, "validate", repackagedPath); code != 0 {
		t.Fatalf("validate of repackaged output failed: code=%d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	validateAgainstSchema(t, repackagedPath)
}

// TestCLIEndToEnd_XLSXInspect checks the xlsx-inspect command against a real XLSX file, which had
// no behavioral test coverage at all before this file.
func TestCLIEndToEnd_XLSXInspect(t *testing.T) {
	input := exampleXLSXPath(t)
	stdout, stderr, code := runCLI(t, "xlsx-inspect", "--json", input)
	if code != 0 {
		t.Fatalf("xlsx-inspect failed: code=%d\nstderr=%s", code, stderr)
	}
	var inspection map[string]any
	if err := json.Unmarshal([]byte(stdout), &inspection); err != nil {
		t.Fatalf("xlsx-inspect did not print valid JSON: %v\n%s", err, stdout)
	}
}

// TestCLIEndToEnd_ValidateRejectsMissingFile checks a real failure path with a real (absent) file
// — the CLI must exit non-zero and say something useful, not silently succeed or panic.
func TestCLIEndToEnd_ValidateRejectsMissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.csvx")
	_, stderr, code := runCLI(t, "validate", missing)
	if code == 0 {
		t.Fatalf("expected non-zero exit for a missing file, got 0")
	}
	if stderr == "" {
		t.Fatalf("expected an error message on stderr for a missing file")
	}
}
