package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// CSV export (csvx-spec 11.2) tested as a CLI: the built binary is run with real arguments and real
// files, and the CSV bytes are compared with the spec's own vectors (tests/export-csv/).

func specPath(t *testing.T, parts ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{"..", "..", "..", "csvx-spec"}, parts...)...)
	if _, err := os.Stat(path); err != nil {
		t.Skipf("csvx-spec checkout not found at %s: %v", path, err)
	}
	return path
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestCLIEndToEnd_ExportCSVDefaultsAndFlags(t *testing.T) {
	typed := specPath(t, "examples", "typed-data.csvx")
	for _, c := range []struct {
		name string
		args []string
		want string
	}{
		{"defaults", nil, "Name,Amount,Date,Active\nAda,19.95,2026-09-22,true\n"},
		{"no header", []string{"--no-header"}, "Ada,19.95,2026-09-22,true\n"},
		{"tab delimiter", []string{"--delimiter", "tab"}, "Name\tAmount\tDate\tActive\nAda\t19.95\t2026-09-22\ttrue\n"},
	} {
		out := filepath.Join(t.TempDir(), "out.csv")
		args := append([]string{"export"}, c.args...)
		if stdout, stderr, code := runCLI(t, append(args, typed, out)...); code != 0 {
			t.Fatalf("%s: export failed: code=%d stdout=%s stderr=%s", c.name, code, stdout, stderr)
		}
		if got := readFileString(t, out); got != c.want {
			t.Errorf("%s: csv = %q; want %q", c.name, got, c.want)
		}
	}
}

func TestCLIEndToEnd_ExportCSVFormulasDisplayAndWarnings(t *testing.T) {
	formulas := specPath(t, "examples", "formulas.csvx")
	out := filepath.Join(t.TempDir(), "values.csv")
	_, stderr, code := runCLI(t, "export", formulas, out)
	if code != 0 {
		t.Fatalf("export failed: stderr=%s", stderr)
	}
	if got, want := readFileString(t, out), "Quantity,Price,Total\n2,4.50,9.00\n3,2.00,6.00\n,,15.00\n"; got != want {
		t.Errorf("formulas as values: csv = %q; want %q", got, want)
	}
	if !strings.Contains(stderr, "warning: formula") {
		t.Errorf("exporting formulas as values must warn that formulas were omitted; stderr=%q", stderr)
	}

	text := filepath.Join(t.TempDir(), "text.csv")
	if _, stderr, code := runCLI(t, "export", "--formulas", "text", formulas, text); code != 0 {
		t.Fatalf("export --formulas text failed: stderr=%s", stderr)
	} else if strings.Contains(stderr, "warning: formula") {
		t.Errorf("formula text omits nothing, so there is no formula warning; stderr=%q", stderr)
	}
	if got, want := readFileString(t, text), "Quantity,Price,Total\n2,4.50,=A2*B2\n3,2.00,=A3*B3\n,,=SUM(C2:C3)\n"; got != want {
		t.Errorf("formulas as text: csv = %q; want %q", got, want)
	}

	styled := filepath.Join(t.TempDir(), "display.csv")
	_, stderr, code = runCLI(t, "export", "--display", specPath(t, "examples", "styled.csvx"), styled)
	if code != 0 {
		t.Fatalf("export --display failed: stderr=%s", stderr)
	}
	if got, want := readFileString(t, styled), "Revenue\n\"$1,250.00\"\nTotal\n"; got != want {
		t.Errorf("display: csv = %q; want %q", got, want)
	}
	if !strings.Contains(stderr, "warning: metadata") {
		t.Errorf("styles are not written, so the export must say so; stderr=%q", stderr)
	}
}

func TestCLIEndToEnd_ExportCSVAllWritesOneFilePerSheet(t *testing.T) {
	multi := specPath(t, "examples", "multi-sheet.csvx")
	dir := filepath.Join(t.TempDir(), "sheets")
	if stdout, stderr, code := runCLI(t, "export", "--all", multi, dir); code != 0 {
		t.Fatalf("export --all failed: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if got, want := readFileString(t, filepath.Join(dir, "Sales.csv")), "Amount\n10.00\n12.50\n"; got != want {
		t.Errorf("Sales.csv = %q; want %q", got, want)
	}
	if got, want := readFileString(t, filepath.Join(dir, "Summary.csv")), "Total\n22.50\n"; got != want {
		t.Errorf("Summary.csv = %q; want %q", got, want)
	}
	single := filepath.Join(t.TempDir(), "summary.csv")
	if _, stderr, code := runCLI(t, "export", "--sheet", "Summary", multi, single); code != 0 {
		t.Fatalf("export --sheet failed: stderr=%s", stderr)
	}
	if got := readFileString(t, single); got != "Total\n22.50\n" {
		t.Errorf("--sheet Summary: csv = %q", got)
	}
}

func TestCLIEndToEnd_ExportCSVRejectsBadUse(t *testing.T) {
	typed := specPath(t, "examples", "typed-data.csvx")
	tempDir := t.TempDir()
	for _, c := range []struct {
		name string
		args []string
	}{
		{"unknown sheet", []string{"export", "--sheet", "Nope", typed, filepath.Join(tempDir, "a.csv")}},
		{"bad delimiter", []string{"export", "--delimiter", "ab", typed, filepath.Join(tempDir, "b.csv")}},
		{"bad formulas value", []string{"export", "--formulas", "both", typed, filepath.Join(tempDir, "c.csv")}},
		{"csv flag with xlsx output", []string{"export", "--no-header", typed, filepath.Join(tempDir, "d.xlsx")}},
		{"all with a csv file", []string{"export", "--all", typed, filepath.Join(tempDir, "e.csv")}},
		{"unknown extension", []string{"export", typed, filepath.Join(tempDir, "f.txt")}},
	} {
		if _, stderr, code := runCLI(t, c.args...); code == 0 || stderr == "" {
			t.Errorf("%s: want a non-zero exit and a message, got code=%d stderr=%q", c.name, code, stderr)
		}
	}
}

// Importing a CSV and exporting it again returns the canonical form pinned by the spec's vectors
// (tests/export-csv/round-trip.json): the check that a CSV survives CSVX unchanged.
func TestCLIEndToEnd_CSVRoundTripMatchesTheSpecVectors(t *testing.T) {
	var vector struct {
		Cases []struct {
			Note  string `json:"note"`
			Input struct {
				File          string `json:"file"`
				ImportOptions struct {
					Header *bool `json:"header"`
					Infer  bool  `json:"infer"`
				} `json:"importOptions"`
			} `json:"input"`
			Expected struct {
				CSV string `json:"csv"`
			} `json:"expected"`
		} `json:"cases"`
	}
	if err := json.Unmarshal([]byte(readFileString(t, specPath(t, "tests", "export-csv", "round-trip.json"))), &vector); err != nil {
		t.Fatal(err)
	}
	for _, c := range vector.Cases {
		tempDir := t.TempDir()
		args := []string{"import"}
		if h := c.Input.ImportOptions.Header; h != nil && !*h {
			args = append(args, "--no-header")
		}
		if c.Input.ImportOptions.Infer {
			args = append(args, "--infer")
		}
		packaged := filepath.Join(tempDir, "book.csvx")
		if _, stderr, code := runCLI(t, append(args, specPath(t, c.Input.File), packaged)...); code != 0 {
			t.Fatalf("%s: import failed: stderr=%s", c.Note, stderr)
		}
		out := filepath.Join(tempDir, "out.csv")
		if _, stderr, code := runCLI(t, "export", packaged, out); code != 0 {
			t.Fatalf("%s: export failed: stderr=%s", c.Note, stderr)
		}
		if got := readFileString(t, out); got != c.Expected.CSV {
			t.Errorf("%s:\n got %q\nwant %q", c.Note, got, c.Expected.CSV)
		}
	}
}
