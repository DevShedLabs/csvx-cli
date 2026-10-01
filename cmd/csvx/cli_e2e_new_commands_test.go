package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// This file tests create/import/export/codegen/gen as CLIs, the same way cli_e2e_test.go tests the
// original commands: build the real binary, invoke it as a subprocess with real arguments and real
// files, and validate real output against the canonical schema validator (csvx-spec/AGENTS.md rule
// 3.7) rather than only checking that internal functions return no error.

func TestCLIEndToEnd_CreateProducesValidPackage(t *testing.T) {
	tempDir := t.TempDir()

	dirOutput := filepath.Join(tempDir, "newbook")
	if _, stderr, code := runCLI(t, "create", dirOutput); code != 0 {
		t.Fatalf("create (directory) failed: code=%d\nstderr=%s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(dirOutput, "manifest.json")); err != nil {
		t.Fatalf("create did not produce manifest.json: %v", err)
	}
	if stdout, stderr, code := runCLI(t, "validate", dirOutput); code != 0 {
		t.Fatalf("validate of created directory failed: code=%d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	zipOutput := filepath.Join(tempDir, "newbook.csvx")
	if _, stderr, code := runCLI(t, "create", "--sheet", "My Data", zipOutput); code != 0 {
		t.Fatalf("create (.csvx) failed: code=%d\nstderr=%s", code, stderr)
	}
	if stdout, stderr, code := runCLI(t, "validate", zipOutput); code != 0 {
		t.Fatalf("validate of created package failed: code=%d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	validateAgainstSchema(t, zipOutput)
}

func TestCLIEndToEnd_CreateRejectsExistingDirectory(t *testing.T) {
	tempDir := t.TempDir()
	target := filepath.Join(tempDir, "exists")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, stderr, code := runCLI(t, "create", target); code == 0 {
		t.Fatalf("expected create to refuse an existing directory, got exit 0 (stderr=%s)", stderr)
	}
}

func TestCLIEndToEnd_ImportAndExport(t *testing.T) {
	input := exampleXLSXPath(t)
	tempDir := t.TempDir()
	importedPath := filepath.Join(tempDir, "imported.csvx")

	if stdout, stderr, code := runCLI(t, "import", input, importedPath); code != 0 {
		t.Fatalf("import failed: code=%d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if stdout, stderr, code := runCLI(t, "validate", importedPath); code != 0 {
		t.Fatalf("validate of imported package failed: code=%d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	validateAgainstSchema(t, importedPath)

	recoveredPath := filepath.Join(tempDir, "recovered.xlsx")
	if stdout, stderr, code := runCLI(t, "export", importedPath, recoveredPath); code != 0 {
		t.Fatalf("export failed: code=%d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if _, err := os.Stat(recoveredPath); err != nil {
		t.Fatalf("export did not produce output file: %v", err)
	}
}

func TestCLIEndToEnd_ImportRejectsWrongDirection(t *testing.T) {
	input := exampleXLSXPath(t)
	tempDir := t.TempDir()
	// import expects .xlsx -> .csvx; giving it the reverse extension pairing must fail clearly,
	// not silently do the wrong thing or produce a confusing error from the generic converter.
	_, stderr, code := runCLI(t, "import", input, filepath.Join(tempDir, "wrong.xlsx"))
	if code == 0 {
		t.Fatalf("expected import to reject a non-.csvx output, got exit 0")
	}
	if stderr == "" {
		t.Fatalf("expected an error message for the wrong output extension")
	}
}

func TestCLIEndToEnd_ExportFailsWithoutEmbeddedSource(t *testing.T) {
	tempDir := t.TempDir()
	created := filepath.Join(tempDir, "nosource.csvx")
	if _, stderr, code := runCLI(t, "create", created); code != 0 {
		t.Fatalf("setup create failed: code=%d\nstderr=%s", code, stderr)
	}
	_, stderr, code := runCLI(t, "export", created, filepath.Join(tempDir, "out.xlsx"))
	if code == 0 {
		t.Fatalf("expected export to fail for a package with no embedded XLSX source")
	}
	if stderr == "" {
		t.Fatalf("expected an explanatory error on stderr")
	}
}

func TestCLIEndToEnd_GenTestCSVXProducesSchemaValidFixture(t *testing.T) {
	tempDir := t.TempDir()
	output := filepath.Join(tempDir, "fixture.csvx")

	if stdout, stderr, code := runCLI(t, "gen", "test.csvx", "--output", output); code != 0 {
		t.Fatalf("gen test.csvx failed: code=%d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if stdout, stderr, code := runCLI(t, "validate", output); code != 0 {
		t.Fatalf("validate of generated fixture failed: code=%d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	validateAgainstSchema(t, output)

	workbook, err := openInput(output)
	if err != nil {
		t.Fatalf("open generated fixture: %v", err)
	}
	wantSheets := map[string]bool{"types": false, "errors": false, "formulas": false, "validation": false}
	for _, sheet := range workbook.Sheets {
		if _, ok := wantSheets[sheet.ID]; ok {
			wantSheets[sheet.ID] = true
		}
	}
	for id, found := range wantSheets {
		if !found {
			t.Errorf("generated fixture missing expected sheet %q", id)
		}
	}
	if len(workbook.Styles) == 0 {
		t.Errorf("generated fixture has no styles")
	}
}

// TestCLIEndToEnd_CodegenReproducesCSVXGoModel is the regression test for the exact drift
// csvx-spec/AGENTS.md describes: csvx-go's internal/schema/generated.go must be reproducible from
// csvx-spec/schemas by running this command, not something a human regenerates by hand and might
// forget. It skips if no sibling csvx-go checkout is present, same convention as the XLSX fixture
// skip in cli_e2e_test.go.
func TestCLIEndToEnd_CodegenReproducesCSVXGoModel(t *testing.T) {
	schemaDir := filepath.Join("..", "..", "..", "csvx-spec", "schemas")
	if _, err := os.Stat(schemaDir); err != nil {
		t.Skipf("csvx-spec schemas not found at %s (expected a sibling checkout): %v", schemaDir, err)
	}
	existing := filepath.Join("..", "..", "..", "csvx-go", "internal", "schema", "generated.go")
	wantBytes, err := os.ReadFile(existing)
	if err != nil {
		t.Skipf("csvx-go generated model not found at %s (expected a sibling checkout): %v", existing, err)
	}
	if _, err := exec.LookPath("go-jsonschema"); err != nil {
		t.Skip("go-jsonschema not installed on PATH")
	}

	tempDir := t.TempDir()
	output := filepath.Join(tempDir, "generated.go")
	stdout, stderr, code := runCLI(t, "codegen", "--lang", "go", "--schema-dir", schemaDir, "--out", output)
	if code != 0 {
		t.Fatalf("codegen failed: code=%d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	gotBytes, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read codegen output: %v", err)
	}
	if string(gotBytes) != string(wantBytes) {
		t.Fatalf("codegen output does not match csvx-go's existing generated.go byte-for-byte — either\n" +
			"schemas/ changed and csvx-go needs regenerating, or codegen's invocation drifted from the one\n" +
			"that produced the existing file")
	}
}
