package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	wantSheets := map[string]bool{"types": false, "errors": false, "formulas": false, "validation": false, "print": false}
	for _, sheet := range workbook.Sheets {
		if _, ok := wantSheets[sheet.ID]; ok {
			wantSheets[sheet.ID] = true
		}
	}
	for _, sheet := range workbook.Sheets {
		if sheet.ID != "print" {
			continue
		}
		if sheet.Print == nil || sheet.Print.Orientation == nil || string(*sheet.Print.Orientation) != "landscape" ||
			sheet.Print.FitToWidth == nil || *sheet.Print.FitToWidth != 1 || len(sheet.Print.ColumnBreaks) != 1 {
			t.Errorf("generated print sheet lost its print settings: %+v", sheet.Print)
		}
		if extra, _ := sheet.Print.AdditionalProperties.(map[string]any); extra["headerFooter"] == nil {
			t.Errorf("generated print sheet lost its unknown headerFooter property")
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

// TestCLIEndToEnd_CodegenTSProducesValidTypeScript checks --lang ts end to end: it must produce a
// file that actually type-checks (not just "ran without error"), and running it twice must produce
// byte-identical output — the dedup logic in splitTopLevelDeclarations is the part most likely to
// regress silently (e.g. two schemas disagreeing about a shared type would otherwise produce
// duplicate, non-identical declarations that fail to compile). Skips without a sibling csvx-spec
// checkout or without json2ts/tsc on PATH, matching the skip convention used elsewhere in this file.
func TestCLIEndToEnd_CodegenTSProducesValidTypeScript(t *testing.T) {
	schemaDir := filepath.Join("..", "..", "..", "csvx-spec", "schemas")
	if _, err := os.Stat(schemaDir); err != nil {
		t.Skipf("csvx-spec schemas not found at %s (expected a sibling checkout): %v", schemaDir, err)
	}
	if _, err := exec.LookPath("json2ts"); err != nil {
		t.Skip("json2ts not installed on PATH (npm install -g json-schema-to-typescript)")
	}

	tempDir := t.TempDir()
	firstOutput := filepath.Join(tempDir, "generated.ts")
	stdout, stderr, code := runCLI(t, "codegen", "--lang", "ts", "--schema-dir", schemaDir, "--out", firstOutput)
	if code != 0 {
		t.Fatalf("codegen --lang ts failed: code=%d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	firstBytes, err := os.ReadFile(firstOutput)
	if err != nil {
		t.Fatalf("read codegen output: %v", err)
	}

	secondOutput := filepath.Join(tempDir, "generated-again.ts")
	if _, stderr, code := runCLI(t, "codegen", "--lang", "ts", "--schema-dir", schemaDir, "--out", secondOutput); code != 0 {
		t.Fatalf("second codegen --lang ts run failed: code=%d\nstderr=%s", code, stderr)
	}
	secondBytes, err := os.ReadFile(secondOutput)
	if err != nil {
		t.Fatalf("read second codegen output: %v", err)
	}
	if string(firstBytes) != string(secondBytes) {
		t.Fatalf("codegen --lang ts is not deterministic: two runs over the same schemas produced different output")
	}

	for _, want := range []string{"CSVXManifest", "CSVXWorkbook", "CSVXStyles", "CSVXSheetMetadata", "CSVXFormulaCell", "interface Value", "type Id ="} {
		if !strings.Contains(string(firstBytes), want) {
			t.Errorf("generated TypeScript missing expected declaration containing %q", want)
		}
	}

	if tscPath, err := exec.LookPath("tsc"); err == nil {
		cmd := exec.Command(tscPath, "--noEmit", "--strict", firstOutput)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("generated TypeScript failed to type-check: %v\n%s", err, output)
		}
	} else {
		t.Log("tsc not on PATH; skipped type-checking the generated file (ran and compared output only)")
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

func csvFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "csvx-spec", "examples", "csv", name)
	if _, err := os.Stat(path); err != nil {
		t.Skipf("csvx-spec CSV fixture not found at %s: %v", path, err)
	}
	return path
}

func TestCLIEndToEnd_ImportCSV(t *testing.T) {
	out := filepath.Join(t.TempDir(), "people.csvx")
	stdout, stderr, code := runCLI(t, "import", "--infer", csvFixture(t, "people.csv"), out)
	if code != 0 {
		t.Fatalf("import csv failed: code=%d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "imported:") || stderr != "" {
		t.Fatalf("unexpected output: stdout=%q stderr=%q", stdout, stderr)
	}
	if stdout, stderr, code := runCLI(t, "validate", out); code != 0 {
		t.Fatalf("validate failed: code=%d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	validateAgainstSchema(t, out)
	// Declared types must have reached the sidecar.
	extracted := filepath.Join(t.TempDir(), "x")
	if _, stderr, code := runCLI(t, "extract", out, "-o", extracted); code != 0 {
		t.Fatalf("extract failed: %s", stderr)
	}
	meta, err := os.ReadFile(filepath.Join(extracted, "sheets", "people.meta.json"))
	if err != nil || !strings.Contains(string(meta), `"decimal"`) || !strings.Contains(string(meta), `"date"`) {
		t.Fatalf("expected inferred types in sidecar, got %v %s", err, meta)
	}
}

func TestCLIEndToEnd_ImportCSVWarnsAndFails(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := runCLI(t, "import", "--name", "Ragged Data", csvFixture(t, "ragged.csv"), filepath.Join(dir, "r.csvx"))
	if code != 0 || !strings.Contains(stderr, "warning: record 2") || !strings.Contains(stderr, "warning: record 3") {
		t.Fatalf("expected success with warnings on stderr, got code=%d stderr=%q", code, stderr)
	}
	_, stderr, code = runCLI(t, "import", csvFixture(t, "malformed.csv"), filepath.Join(dir, "m.csvx"))
	if code != 1 || !strings.Contains(stderr, "line 2") {
		t.Fatalf("expected exit 1 naming line 2, got code=%d stderr=%q", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "m.csvx")); err == nil {
		t.Fatalf("failed import must not leave an output file")
	}
	_, _, code = runCLI(t, "import", "--delimiter", "ab", csvFixture(t, "people.csv"), filepath.Join(dir, "d.csvx"))
	if code != 2 {
		t.Fatalf("expected usage error (2) for a multi-character delimiter, got %d", code)
	}
}
