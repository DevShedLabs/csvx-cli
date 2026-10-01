package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	csvx "github.com/DevShedLabs/csvx-go"
)

// import and export are the first-class names csvx-spec/AGENTS.md rule 4 gives these two
// directions (distinct from the lower-level `convert`, which is kept for backward compatibility
// and still auto-detects direction from file extensions). Naming them explicitly means a user who
// means "take this XLSX and give me CSVX" doesn't have to know `convert` happens to do that when
// given a .xlsx/.csvx pair — and each one validates its own direction so a swapped argument order
// fails with a clear message instead of a confusing one from the generic converter.

func runImport(arguments []string) {
	input, output, showHelp, err := parseConvertArguments(arguments)
	if err != nil {
		fmt.Fprintf(os.Stderr, "csvx import: %v\n\n", err)
		printCommandHelp(os.Stderr, "import")
		os.Exit(2)
	}
	if showHelp {
		printCommandHelp(os.Stdout, "import")
		return
	}
	if ext := strings.ToLower(filepath.Ext(input)); ext != ".xlsx" {
		fmt.Fprintf(os.Stderr, "csvx import: expected an .xlsx input, got %q\n", input)
		os.Exit(2)
	}
	if ext := strings.ToLower(filepath.Ext(output)); ext != ".csvx" {
		fmt.Fprintf(os.Stderr, "csvx import: expected a .csvx output, got %q\n", output)
		os.Exit(2)
	}
	if err := csvx.Convert(input, output); err != nil {
		fmt.Fprintf(os.Stderr, "csvx import: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("imported: %s\n", output)
}

func runExport(arguments []string) {
	input, output, showHelp, err := parseConvertArguments(arguments)
	if err != nil {
		fmt.Fprintf(os.Stderr, "csvx export: %v\n\n", err)
		printCommandHelp(os.Stderr, "export")
		os.Exit(2)
	}
	if showHelp {
		printCommandHelp(os.Stdout, "export")
		return
	}
	if ext := strings.ToLower(filepath.Ext(input)); ext != ".csvx" {
		fmt.Fprintf(os.Stderr, "csvx export: expected a .csvx input, got %q\n", input)
		os.Exit(2)
	}
	if ext := strings.ToLower(filepath.Ext(output)); ext != ".xlsx" {
		fmt.Fprintf(os.Stderr, "csvx export: expected an .xlsx output, got %q\n", output)
		os.Exit(2)
	}
	if err := csvx.Convert(input, output); err != nil {
		// csvx-go only recovers an unmodified embedded original today (csvx-spec/AGENTS.md rule
		// 4.5) — general export of an arbitrary/edited workbook doesn't exist yet. Say so rather
		// than letting the underlying error stand alone and look like a bug in this command.
		fmt.Fprintf(os.Stderr, "csvx export: %v\n(note: only recovery of an unmodified embedded XLSX source is supported today; exporting an edited/arbitrary CSVX workbook to XLSX is not yet implemented — see csvx-spec/AGENTS.md rule 4.5)\n", err)
		os.Exit(1)
	}
	fmt.Printf("exported: %s\n", output)
}
