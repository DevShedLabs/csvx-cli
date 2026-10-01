package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	csvx "github.com/DevShedLabs/csvx-go"
)

// create scaffolds a new, minimal, schema-valid CSVX package — either an unpacked directory or a
// packaged .csvx file, detected from the output path's extension, matching how every other command
// here treats .csvx/directory as interchangeable. The shape it writes mirrors
// csvx-spec/examples/minimal.csvx exactly, since that golden fixture is already known-valid; this
// is intentionally not re-derived by constructing csvx.Workbook/csvx.Sheet values and calling
// csvx.WritePackage, which would duplicate decisions already made by the spec's own golden example.
func runCreate(arguments []string) {
	output, sheetName, showHelp, err := parseCreateArguments(arguments)
	if err != nil {
		fmt.Fprintf(os.Stderr, "csvx create: %v\n\n", err)
		printCommandHelp(os.Stderr, "create")
		os.Exit(2)
	}
	if showHelp {
		printCommandHelp(os.Stdout, "create")
		return
	}
	if err := createPackage(output, sheetName); err != nil {
		fmt.Fprintf(os.Stderr, "csvx create: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("created: %s\n", output)
}

func parseCreateArguments(arguments []string) (output string, sheetName string, help bool, err error) {
	sheetName = "Sheet 1"
	for index := 0; index < len(arguments); index++ {
		switch arguments[index] {
		case "--help", "-h":
			return "", "", true, nil
		case "--sheet":
			if index+1 >= len(arguments) {
				return "", "", false, fmt.Errorf("--sheet requires a name")
			}
			sheetName = arguments[index+1]
			index++
		default:
			if output != "" {
				return "", "", false, fmt.Errorf("expected one output path, got %q", arguments[index])
			}
			output = arguments[index]
		}
	}
	if output == "" {
		return "", "", false, fmt.Errorf("provide an output path (a directory, or a file ending in .csvx)")
	}
	return output, sheetName, false, nil
}

func createPackage(output, sheetName string) error {
	packageAsZip := strings.EqualFold(filepath.Ext(output), ".csvx")
	workDir := output
	if packageAsZip {
		stagingDir, err := os.MkdirTemp("", "csvx-create-*")
		if err != nil {
			return fmt.Errorf("create staging directory: %w", err)
		}
		defer os.RemoveAll(stagingDir)
		workDir = stagingDir
	} else {
		if _, err := os.Stat(output); err == nil {
			return fmt.Errorf("output directory already exists: %s", output)
		}
	}

	if err := writeMinimalPackage(workDir, sheetName); err != nil {
		return err
	}

	if packageAsZip {
		return csvx.PackageDirectory(workDir, output)
	}
	return nil
}

func writeMinimalPackage(directory, sheetName string) error {
	sheetsDir := filepath.Join(directory, "sheets")
	if err := os.MkdirAll(sheetsDir, 0o755); err != nil {
		return fmt.Errorf("create sheets directory: %w", err)
	}

	manifest := `{
	"format": "csvx",
	"version": "1.0",
	"workbook": "workbook.json",
	"files": [
		"manifest.json",
		"workbook.json",
		"sheets/sheet-1.csv"
	]
}`
	workbook := fmt.Sprintf(`{
	"id": "book-1",
	"version": "1.0",
	"sheets": [
		{
			"id": "sheet-1",
			"name": %q,
			"path": "sheets/sheet-1.csv"
		}
	],
	"calculation": {
		"mode": "automatic",
		"iteration": false
	}
}`, sheetName)
	sheet := "Value\n"

	writes := map[string]string{
		"manifest.json":      manifest,
		"workbook.json":      workbook,
		"sheets/sheet-1.csv": sheet,
	}
	for relativePath, contents := range writes {
		fullPath := filepath.Join(directory, filepath.FromSlash(relativePath))
		if err := os.WriteFile(fullPath, []byte(contents), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", relativePath, err)
		}
	}
	return nil
}
