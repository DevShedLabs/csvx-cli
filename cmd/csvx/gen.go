package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	csvx "github.com/DevShedLabs/csvx-go"
)

// gen test.csvx is the schema-exhaustive fixture generator csvx-spec/AGENTS.md rule 4.4 requires:
// coverage derived directly from schemas/*.json and spec/04-data-types.md (every scalar type,
// every style property, multi-sheet, formulas, validation rules) rather than copying the
// hand-authored examples/, which are illustrative and intentionally small. AGENTS.md rule 4.5 also
// calls for a `gen test.xlsx` counterpart with round-trip interop tests — that's real work tracked
// separately (it needs a general CSVX→XLSX exporter first, which doesn't exist yet) and is not
// built here.
func runGen(arguments []string) {
	if len(arguments) == 0 {
		fmt.Fprintln(os.Stderr, "csvx gen: expected a target (test.csvx)")
		printCommandHelp(os.Stderr, "gen")
		os.Exit(2)
	}
	target := arguments[0]
	rest := arguments[1:]
	switch target {
	case "test.csvx":
		runGenTestCSVX(rest)
	case "--help", "-h":
		printCommandHelp(os.Stdout, "gen")
	default:
		fmt.Fprintf(os.Stderr, "csvx gen: unknown target %q (only \"test.csvx\" is implemented)\n", target)
		os.Exit(2)
	}
}

func runGenTestCSVX(arguments []string) {
	output, help, err := parseGenTestCSVXArguments(arguments)
	if err != nil {
		fmt.Fprintf(os.Stderr, "csvx gen test.csvx: %v\n\n", err)
		printCommandHelp(os.Stderr, "gen")
		os.Exit(2)
	}
	if help {
		printCommandHelp(os.Stdout, "gen")
		return
	}
	if err := genTestCSVX(output); err != nil {
		fmt.Fprintf(os.Stderr, "csvx gen test.csvx: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("generated: %s\n", output)
}

func parseGenTestCSVXArguments(arguments []string) (output string, help bool, err error) {
	for index := 0; index < len(arguments); index++ {
		switch arguments[index] {
		case "--help", "-h":
			return "", true, nil
		case "--output", "-o":
			if index+1 >= len(arguments) {
				return "", false, fmt.Errorf("--output requires a path")
			}
			output = arguments[index+1]
			index++
		default:
			return "", false, fmt.Errorf("unexpected argument %q", arguments[index])
		}
	}
	if output == "" {
		output = "test.csvx"
	}
	return output, false, nil
}

// scalarTypeSamples mirrors the nine core scalar types from csvx-spec/spec/04-data-types.md
// verbatim, each with a representative value in both raw CSV form and its typed JSON form.
var scalarTypeSamples = []struct {
	typeName string
	csvValue string
}{
	{"blank", ""},
	{"boolean", "true"},
	{"integer", "42"},
	{"decimal", "19.95"},
	{"string", "hello"},
	{"date", "2026-09-22"},
	{"time", "12:30:00"},
	{"datetime", "2026-09-22T12:30:00Z"},
	{"error", "#DIV/0!"},
}

// coreErrorCodes mirrors spec/04-data-types.md's "Core error codes" list verbatim.
var coreErrorCodes = []string{"NULL", "DIV0", "VALUE", "REF", "NAME", "NUM", "N/A", "CYCLE"}

// validationTypes mirrors the sheet-metadata.schema.json validation.type enum verbatim.
var validationTypes = []string{"whole", "decimal", "list", "date", "textLength", "custom"}

func genTestCSVX(output string) error {
	stagingDir, err := os.MkdirTemp("", "csvx-gen-test-*")
	if err != nil {
		return fmt.Errorf("create staging directory: %w", err)
	}
	defer os.RemoveAll(stagingDir)

	sheetsDir := filepath.Join(stagingDir, "sheets")
	if err := os.MkdirAll(sheetsDir, 0o755); err != nil {
		return fmt.Errorf("create sheets directory: %w", err)
	}

	if err := writeTypesSheet(sheetsDir); err != nil {
		return err
	}
	if err := writeErrorsSheet(sheetsDir); err != nil {
		return err
	}
	if err := writeFormulasSheet(sheetsDir); err != nil {
		return err
	}
	if err := writeValidationSheet(sheetsDir); err != nil {
		return err
	}
	if err := writePrintSheet(sheetsDir); err != nil {
		return err
	}
	if err := writeStylesFile(stagingDir); err != nil {
		return err
	}
	if err := writeGenManifestAndWorkbook(stagingDir); err != nil {
		return err
	}

	if strings.EqualFold(filepath.Ext(output), ".csvx") {
		return csvx.PackageDirectory(stagingDir, output)
	}
	return copyDirectory(stagingDir, output)
}

// writeTypesSheet exercises every scalar type in a single column, one row per type, named after
// the type so the fixture is self-documenting.
func writeTypesSheet(sheetsDir string) error {
	var csvLines []string
	csvLines = append(csvLines, "TypeName,Value")
	for _, sample := range scalarTypeSamples {
		csvLines = append(csvLines, fmt.Sprintf("%s,%s", sample.typeName, csvEscape(sample.csvValue)))
	}
	if err := os.WriteFile(filepath.Join(sheetsDir, "types.csv"), []byte(strings.Join(csvLines, "\n")+"\n"), 0o644); err != nil {
		return err
	}

	var cells strings.Builder
	cells.WriteString(`{
	"id": "types",
	"name": "Types",
	"columns": [
		{"id": "A", "name": "TypeName", "type": "string"},
		{"id": "B", "name": "Value", "type": "string"}
	],
	"cells": {
`)
	for index, sample := range scalarTypeSamples {
		row := index + 2
		comma := ","
		if index == len(scalarTypeSamples)-1 {
			comma = ""
		}
		fmt.Fprintf(&cells, "\t\t\"B%d\": {\"type\": %q}%s\n", row, sample.typeName, comma)
	}
	cells.WriteString("\t}\n}")
	return os.WriteFile(filepath.Join(sheetsDir, "types.meta.json"), []byte(cells.String()), 0o644)
}

// writeErrorsSheet exercises every core error code from spec/04-data-types.md, one per row.
func writeErrorsSheet(sheetsDir string) error {
	var csvLines []string
	csvLines = append(csvLines, "Code,Value")
	for _, code := range coreErrorCodes {
		csvLines = append(csvLines, fmt.Sprintf("%s,#ERR", code))
	}
	if err := os.WriteFile(filepath.Join(sheetsDir, "errors.csv"), []byte(strings.Join(csvLines, "\n")+"\n"), 0o644); err != nil {
		return err
	}

	var cells strings.Builder
	cells.WriteString(`{
	"id": "errors",
	"name": "Errors",
	"columns": [
		{"id": "A", "name": "Code", "type": "string"},
		{"id": "B", "name": "Value", "type": "error"}
	],
	"cells": {
`)
	for index, code := range coreErrorCodes {
		row := index + 2
		comma := ","
		if index == len(coreErrorCodes)-1 {
			comma = ""
		}
		fmt.Fprintf(&cells, "\t\t\"B%d\": {\"type\": \"error\", \"code\": %q}%s\n", row, code, comma)
	}
	cells.WriteString("\t}\n}")
	return os.WriteFile(filepath.Join(sheetsDir, "errors.meta.json"), []byte(cells.String()), 0o644)
}

// writeFormulasSheet gives a representative formula set: an arithmetic formula per row plus a
// SUM aggregate, each with a cached decimal value, matching the shape of
// csvx-spec/examples/formulas.csvx.
func writeFormulasSheet(sheetsDir string) error {
	csv := "Quantity,Price,Total\n2,4.50,\n3,2.00,\n,,\n"
	if err := os.WriteFile(filepath.Join(sheetsDir, "formulas.csv"), []byte(csv), 0o644); err != nil {
		return err
	}
	meta := `{
	"id": "formulas",
	"name": "Formulas",
	"columns": [
		{"id": "A", "name": "Quantity", "type": "integer"},
		{"id": "B", "name": "Price", "type": "decimal"},
		{"id": "C", "name": "Total", "type": "decimal"}
	],
	"cells": {
		"C2": {"formula": "=A2*B2", "cached": {"type": "decimal", "value": "9.00"}},
		"C3": {"formula": "=A3*B3", "cached": {"type": "decimal", "value": "6.00"}},
		"C4": {"formula": "=SUM(C2:C3)", "cached": {"type": "decimal", "value": "15.00"}}
	}
}`
	return os.WriteFile(filepath.Join(sheetsDir, "formulas.meta.json"), []byte(meta), 0o644)
}

// writePrintSheet exercises every property of the sheet `print` object from
// sheet-metadata.schema.json (spec/03-sheets.md, "Print settings"), plus one property the schema
// doesn't define (headerFooter), which the spec requires engines to preserve.
func writePrintSheet(sheetsDir string) error {
	csv := "Region,Q1,Q2,Q3,Q4\nNorth,10,12,14,16\nSouth,9,11,13,15\n"
	if err := os.WriteFile(filepath.Join(sheetsDir, "print.csv"), []byte(csv), 0o644); err != nil {
		return err
	}
	meta := `{
	"id": "print",
	"name": "Print",
	"print": {
		"orientation": "landscape",
		"paperSize": "a4",
		"margins": {"top": 0.5, "right": 0.4, "bottom": 0.6, "left": 0.3},
		"scale": 90,
		"fitToWidth": 1,
		"fitToHeight": 0,
		"area": "A1:E3",
		"repeatRows": "1:1",
		"repeatColumns": "A:A",
		"pageOrder": "overThenDown",
		"gridlines": true,
		"centerHorizontally": true,
		"columnBreaks": [3],
		"rowBreaks": [2],
		"headerFooter": {"oddFooter": "&P of &N"}
	}
}`
	return os.WriteFile(filepath.Join(sheetsDir, "print.meta.json"), []byte(meta), 0o644)
}

// writeValidationSheet exercises every validation.type from sheet-metadata.schema.json, one per
// row, each with a representative operator/formula pairing.
func writeValidationSheet(sheetsDir string) error {
	var csvLines []string
	csvLines = append(csvLines, "Rule,Value")
	for _, kind := range validationTypes {
		csvLines = append(csvLines, fmt.Sprintf("%s,", kind))
	}
	if err := os.WriteFile(filepath.Join(sheetsDir, "validation.csv"), []byte(strings.Join(csvLines, "\n")+"\n"), 0o644); err != nil {
		return err
	}

	rules := map[string]string{
		"whole":      `{"type": "whole", "operator": "between", "formula1": "1", "formula2": "10"}`,
		"decimal":    `{"type": "decimal", "operator": "greaterThan", "formula1": "0"}`,
		"list":       `{"type": "list", "formula1": "\"Yes,No,Maybe\""}`,
		"date":       `{"type": "date", "operator": "greaterThan", "formula1": "2026-01-01"}`,
		"textLength": `{"type": "textLength", "operator": "lessThanOrEqual", "formula1": "255"}`,
		"custom":     `{"type": "custom", "formula1": "=B2<>\"\""}`,
	}
	var cells strings.Builder
	cells.WriteString("{\n\t\"id\": \"validation\",\n\t\"name\": \"Validation\",\n\t\"columns\": [\n\t\t{\"id\": \"A\", \"name\": \"Rule\", \"type\": \"string\"},\n\t\t{\"id\": \"B\", \"name\": \"Value\", \"type\": \"string\"}\n\t],\n\t\"cells\": {\n")
	for index, kind := range validationTypes {
		row := index + 2
		comma := ","
		if index == len(validationTypes)-1 {
			comma = ""
		}
		fmt.Fprintf(&cells, "\t\t\"B%d\": {\"validation\": %s}%s\n", row, rules[kind], comma)
	}
	cells.WriteString("\t}\n}")
	return os.WriteFile(filepath.Join(sheetsDir, "validation.meta.json"), []byte(cells.String()), 0o644)
}

// writeStylesFile exercises every style property category from styles.schema.json (numberFormat,
// font, fill, border, alignment, protection), both individually and combined, matching the
// array-of-self-identifying-objects shape styles.schema.json requires.
func writeStylesFile(stagingDir string) error {
	styles := `{
	"styles": [
		{"id": "numberFormat", "numberFormat": "$#,##0.00"},
		{"id": "font", "font": {"bold": true, "italic": true, "color": "#111111", "size": 12}},
		{"id": "fill", "fill": {"color": "#E8F0FE"}},
		{"id": "border", "border": {"top": {"style": "thin", "color": "#000000"}, "bottom": {"style": "thin", "color": "#000000"}}},
		{"id": "alignment", "alignment": {"horizontal": "right", "vertical": "center"}},
		{"id": "protection", "protection": {"locked": false}},
		{
			"id": "combined",
			"numberFormat": "0.00%",
			"font": {"bold": true},
			"fill": {"color": "#FFF8E1"},
			"border": {"top": {"style": "thin", "color": "#000000"}},
			"alignment": {"horizontal": "center"},
			"protection": {"locked": true}
		}
	]
}`
	return os.WriteFile(filepath.Join(stagingDir, "styles.json"), []byte(styles), 0o644)
}

func writeGenManifestAndWorkbook(stagingDir string) error {
	manifest := `{
	"format": "csvx",
	"version": "1.0",
	"workbook": "workbook.json",
	"files": [
		"manifest.json",
		"workbook.json",
		"styles.json",
		"sheets/types.csv",
		"sheets/types.meta.json",
		"sheets/errors.csv",
		"sheets/errors.meta.json",
		"sheets/formulas.csv",
		"sheets/formulas.meta.json",
		"sheets/validation.csv",
		"sheets/validation.meta.json",
		"sheets/print.csv",
		"sheets/print.meta.json"
	]
}`
	workbook := `{
	"id": "test-fixture",
	"version": "1.0",
	"sheets": [
		{"id": "types", "name": "Types", "path": "sheets/types.csv", "metadata": "sheets/types.meta.json"},
		{"id": "errors", "name": "Errors", "path": "sheets/errors.csv", "metadata": "sheets/errors.meta.json"},
		{"id": "formulas", "name": "Formulas", "path": "sheets/formulas.csv", "metadata": "sheets/formulas.meta.json"},
		{"id": "validation", "name": "Validation", "path": "sheets/validation.csv", "metadata": "sheets/validation.meta.json"},
		{"id": "print", "name": "Print", "path": "sheets/print.csv", "metadata": "sheets/print.meta.json"}
	],
	"calculation": {
		"mode": "automatic",
		"iteration": false
	}
}`
	if err := os.WriteFile(filepath.Join(stagingDir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(stagingDir, "workbook.json"), []byte(workbook), 0o644)
}

func csvEscape(value string) string {
	if strings.ContainsAny(value, ",\"\n") {
		return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
	}
	return value
}

func copyDirectory(source, destination string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, contents, info.Mode())
	})
}
