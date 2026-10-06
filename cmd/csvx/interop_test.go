package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	csvx "github.com/DevShedLabs/csvx-go"
)

// interopVector mirrors the format-neutral vector shape documented in
// csvx-spec/tests/interop/README.md. This test is the executable runner for the xlsx-to-csvx
// category described there — see that README for why these checks exist (schema validation alone
// does not catch semantic drift, such as a decimal formula result cached as "string").
type interopVector struct {
	ID        string `json:"id"`
	Operation string `json:"operation"`
	Input     string `json:"input"`
	Expected  struct {
		SchemaValid bool             `json:"schemaValid"`
		Samples     []map[string]any `json:"samples"`
	} `json:"expected"`
}

func loadInteropVector(t *testing.T, name string) (interopVector, string) {
	t.Helper()
	vectorPath := filepath.Join("..", "..", "..", "csvx-spec", "tests", "interop", name)
	raw, err := os.ReadFile(vectorPath)
	if err != nil {
		t.Skipf("interop vector not found at %s (expected a sibling csvx-spec checkout): %v", vectorPath, err)
	}
	var vector interopVector
	if err := json.Unmarshal(raw, &vector); err != nil {
		t.Fatalf("parse interop vector %s: %v", name, err)
	}
	return vector, filepath.Dir(vectorPath)
}

// parseA1 converts an A1-style coordinate (e.g. "C11") into a zero-based (column, row) pair.
func parseA1(t *testing.T, ref string) (column, row int) {
	t.Helper()
	split := 0
	for split < len(ref) && ref[split] >= 'A' && ref[split] <= 'Z' {
		split++
	}
	if split == 0 || split == len(ref) {
		t.Fatalf("invalid A1 reference: %q", ref)
	}
	for _, char := range ref[:split] {
		column = column*26 + int(char-'A'+1)
	}
	column--
	rowNumber, err := strconv.Atoi(ref[split:])
	if err != nil {
		t.Fatalf("invalid A1 reference: %q: %v", ref, err)
	}
	return column, rowNumber - 1
}

func findSheetByName(t *testing.T, workbook *csvx.Workbook, name string) *csvx.Sheet {
	t.Helper()
	for _, sheet := range workbook.Sheets {
		if sheet.Name == name {
			return sheet
		}
	}
	t.Fatalf("sheet %q not found", name)
	return nil
}

// validateAgainstSchema calls the canonical schema validator (csvx-spec/validator) on a real
// output file. Per csvx-spec/AGENTS.md rule 3.3, engines and the CLI must not reimplement JSON
// Schema validation themselves — this is the one place that logic runs, and it must actually run
// as part of the test, not be left as a comment for a human to remember.
func validateAgainstSchema(t *testing.T, packagePath string) {
	t.Helper()
	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Fatalf("schema validation requires node on PATH (see csvx-spec/validator/README.md): %v", err)
	}
	validatorDir := filepath.Join("..", "..", "..", "csvx-spec", "validator")
	validatorBin := filepath.Join(validatorDir, "bin", "csvx-validate.mjs")
	if _, err := os.Stat(validatorBin); err != nil {
		t.Skipf("schema validator not found at %s (expected a sibling csvx-spec checkout): %v", validatorBin, err)
	}
	if _, err := os.Stat(filepath.Join(validatorDir, "node_modules")); err != nil {
		t.Fatalf("csvx-spec/validator dependencies not installed — run `npm install` in %s: %v", validatorDir, err)
	}
	absPackagePath, err := filepath.Abs(packagePath)
	if err != nil {
		t.Fatalf("resolve package path: %v", err)
	}
	cmd := exec.Command(nodePath, "bin/csvx-validate.mjs", absPackagePath)
	cmd.Dir = validatorDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("schema validation failed for %s:\n%s", packagePath, output)
	}
}

func findStyleByID(t *testing.T, workbook *csvx.Workbook, id string) csvx.Style {
	t.Helper()
	for _, style := range workbook.Styles {
		if style.Id == id {
			return style
		}
	}
	t.Fatalf("style %q not found", id)
	return csvx.Style{}
}

// TestXLSXToCSVXInteropVector converts the real fixture referenced by
// csvx-spec/tests/interop/xlsx-to-csvx-example.json and checks the semantic assertions in it,
// not just that the output parses. This is what catches drift that schema validation alone can't:
// see csvx-spec/AGENTS.md rule 4.5.
func TestXLSXToCSVXInteropVector(t *testing.T) {
	vector, vectorDir := loadInteropVector(t, "xlsx-to-csvx-example.json")
	if vector.Operation != "xlsx-to-csvx" {
		t.Fatalf("unexpected operation %q", vector.Operation)
	}

	inputPath := filepath.Join(vectorDir, vector.Input)
	outputPath := filepath.Join(t.TempDir(), "output.csvx")
	if err := csvx.Convert(inputPath, outputPath); err != nil {
		t.Fatalf("convert %s: %v", inputPath, err)
	}

	workbook, err := csvx.Open(outputPath)
	if err != nil {
		t.Fatalf("open converted package: %v", err)
	}

	for _, sample := range vector.Expected.Samples {
		sheetName, _ := sample["sheet"].(string)
		cellRef, _ := sample["cell"].(string)
		sheet := findSheetByName(t, workbook, sheetName)
		metadata := sheet.Cells[cellRef]
		label := fmt.Sprintf("%s!%s", sheetName, cellRef)

		for key, want := range sample {
			switch key {
			case "sheet", "cell":
				continue
			case "type":
				if metadata.Type != want {
					t.Errorf("%s: type = %q, want %q", label, metadata.Type, want)
				}
			case "value":
				// Row 1 is the CSV header (spec/03-sheets.md): its value is the column's name. Data
				// row N is Records[N-2].
				column, row := parseA1(t, cellRef)
				var got string
				switch {
				case column < 0 || column >= len(sheet.Columns) || row < 0:
					t.Errorf("%s: cell out of range for raw CSV value", label)
					continue
				case row == 0:
					got = sheet.Columns[column].Name
				case row-1 >= len(sheet.Records) || column >= len(sheet.Records[row-1]):
					t.Errorf("%s: cell out of range for raw CSV value", label)
					continue
				default:
					got = sheet.Records[row-1][column]
				}
				if got != want {
					t.Errorf("%s: raw value = %q, want %q", label, got, want)
				}
			case "formula":
				if metadata.Formula != want {
					t.Errorf("%s: formula = %q, want %q", label, metadata.Formula, want)
				}
			case "cached.type":
				if metadata.Cached == nil {
					t.Errorf("%s: expected cached value, got none", label)
					continue
				}
				if metadata.Cached.Type != want {
					t.Errorf("%s: cached.type = %q, want %q", label, metadata.Cached.Type, want)
				}
			case "cached.value":
				if metadata.Cached == nil {
					t.Errorf("%s: expected cached value, got none", label)
					continue
				}
				if fmt.Sprint(metadata.Cached.Value) != want {
					t.Errorf("%s: cached.value = %v, want %v", label, metadata.Cached.Value, want)
				}
			case "style.font.bold":
				style := findStyleByID(t, workbook, metadata.Style)
				got := style.Font != nil && style.Font.Bold != nil && *style.Font.Bold
				if got != want {
					t.Errorf("%s: style.font.bold = %v, want %v", label, got, want)
				}
			case "style.font.size":
				// Spec 08-styles.md: a number of points, never a string.
				style := findStyleByID(t, workbook, metadata.Style)
				if style.Font == nil || style.Font.Size == nil || *style.Font.Size != want {
					t.Errorf("%s: style.font.size = %v, want %v", label, style.Font, want)
				}
			case "style.fill.color":
				style := findStyleByID(t, workbook, metadata.Style)
				got, _ := style.Fill["color"].(string)
				if got != want {
					t.Errorf("%s: style.fill.color = %q, want %q", label, got, want)
				}
			case "style.numberFormat":
				style := findStyleByID(t, workbook, metadata.Style)
				if style.NumberFormat == nil || *style.NumberFormat != want {
					t.Errorf("%s: style.numberFormat = %v, want %q", label, style.NumberFormat, want)
				}
			default:
				t.Errorf("%s: unhandled sample assertion key %q (add a case in interop_test.go)", label, key)
			}
		}
	}

	if vector.Expected.SchemaValid {
		validateAgainstSchema(t, outputPath)
	}
}
