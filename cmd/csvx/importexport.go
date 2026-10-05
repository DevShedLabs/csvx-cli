package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	csvx "github.com/DevShedLabs/csvx-go"
)

// import and export are the first-class names csvx-spec/AGENTS.md rule 4 gives these two
// directions (distinct from the lower-level `convert`, which is kept for backward compatibility
// and still auto-detects direction from file extensions). Naming them explicitly means a user who
// means "take this XLSX and give me CSVX" doesn't have to know `convert` happens to do that when
// given a .xlsx/.csvx pair — and each one validates its own direction so a swapped argument order
// fails with a clear message instead of a confusing one from the generic converter.

func runImport(arguments []string) {
	if isCSVImport(arguments) {
		runImportCSV(arguments)
		return
	}
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

// exportFlags are the CSV-only options of csvx-spec 11.2, mapped one to one to flags.
type exportFlags struct {
	csvx.CSVExportOptions
	all bool
	set bool // any CSV-only flag was given
}

func parseExportArguments(arguments []string) (input, output string, flags exportFlags, help bool, err error) {
	var positional []string
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		value := func() (string, error) {
			if index+1 >= len(arguments) {
				return "", fmt.Errorf("%s requires a value", argument)
			}
			index++
			return arguments[index], nil
		}
		switch argument {
		case "--help", "-h":
			return "", "", flags, true, nil
		case "--no-header":
			flags.NoHeader, flags.set = true, true
		case "--display":
			flags.Display, flags.set = true, true
		case "--all":
			flags.all, flags.set = true, true
		case "--sheet":
			if flags.Sheet, err = value(); err != nil {
				return "", "", flags, false, err
			}
			flags.set = true
		case "--formulas":
			if flags.Formulas, err = value(); err != nil {
				return "", "", flags, false, err
			}
			if flags.Formulas != "values" && flags.Formulas != "text" {
				return "", "", flags, false, fmt.Errorf("--formulas must be \"values\" or \"text\", got %q", flags.Formulas)
			}
			flags.set = true
		case "--delimiter":
			delimiter, err := value()
			if err != nil {
				return "", "", flags, false, err
			}
			if delimiter == "tab" || delimiter == "\\t" {
				delimiter = "\t"
			}
			if utf8.RuneCountInString(delimiter) != 1 || delimiter == "\"" || delimiter == "\r" || delimiter == "\n" {
				return "", "", flags, false, fmt.Errorf("--delimiter must be a single character other than a double quote, CR or LF (or \"tab\"), got %q", delimiter)
			}
			flags.Delimiter, _ = utf8.DecodeRuneInString(delimiter)
			flags.set = true
		default:
			if strings.HasPrefix(argument, "-") {
				return "", "", flags, false, fmt.Errorf("unknown flag %q", argument)
			}
			positional = append(positional, argument)
		}
	}
	if len(positional) != 2 {
		return "", "", flags, false, fmt.Errorf("provide an input .csvx file and an output file or directory")
	}
	return positional[0], positional[1], flags, false, nil
}

var unsafeFileNameCharacters = regexp.MustCompile(`[\x00-\x1f/\\:*?"<>|]`)

// csvFileNames gives each sheet a safe, unique <name>.csv file name.
func csvFileNames(workbook *csvx.Workbook) []string {
	used := map[string]bool{}
	names := make([]string, len(workbook.Sheets))
	for i, sheet := range workbook.Sheets {
		base := strings.TrimSpace(unsafeFileNameCharacters.ReplaceAllString(sheet.Name, "_"))
		if base == "" || base == "." || base == ".." {
			base = sheet.ID
		}
		name := base
		for n := 2; used[strings.ToLower(name)]; n++ {
			name = fmt.Sprintf("%s_%d", base, n)
		}
		used[strings.ToLower(name)] = true
		names[i] = name + ".csv"
	}
	return names
}

func printExportWarnings(warnings []csvx.ExportWarning) {
	for _, warning := range warnings {
		fmt.Fprintf(os.Stderr, "csvx export: warning: %s: %s: %s\n", warning.Feature, warning.Location, warning.Message)
	}
}

func runExport(arguments []string) {
	input, output, flags, showHelp, err := parseExportArguments(arguments)
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
	outputExt := strings.ToLower(filepath.Ext(output))
	switch {
	case flags.all && outputExt == ".csv", flags.all && outputExt == ".xlsx":
		fmt.Fprintf(os.Stderr, "csvx export: --all writes one CSV per sheet into a directory; got %q\n", output)
		os.Exit(2)
	case flags.all, outputExt == ".csv":
		runExportCSV(input, output, flags)
		return
	case outputExt != ".xlsx":
		fmt.Fprintf(os.Stderr, "csvx export: expected a .xlsx or .csv output (or a directory with --all), got %q\n", output)
		os.Exit(2)
	case flags.set:
		fmt.Fprintf(os.Stderr, "csvx export: --sheet, --no-header, --delimiter, --formulas, --display and --all apply to CSV output only\n")
		os.Exit(2)
	}
	workbook, err := openInput(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "csvx export: %v\n", err)
		os.Exit(1)
	}
	// An unmodified package whose embedded XLSX is still authoritative is recovered exactly
	// (csvx-spec 14.2). Anything else — an edited package, or one that never had a source — is
	// written from its CSVX content (14.9), and the loss warnings the exporter reports are printed.
	if workbook.Source != nil && workbook.Source.Authority == "original" && len(workbook.SourceBytes) > 0 {
		if err := csvx.Convert(input, output); err != nil {
			fmt.Fprintf(os.Stderr, "csvx export: %v\n", err)
			os.Exit(1)
		}
	} else {
		warnings, err := csvx.ExportXLSX(workbook, output)
		if err != nil {
			fmt.Fprintf(os.Stderr, "csvx export: %v\n", err)
			os.Exit(1)
		}
		for _, warning := range warnings {
			fmt.Fprintf(os.Stderr, "csvx export: warning: %s: %s\n", warning.Feature, warning.Message)
		}
	}
	fmt.Printf("exported: %s\n", output)
}

// runExportCSV writes one sheet (or, with --all, every sheet) as CSV, csvx-spec 11.2.
func runExportCSV(input, output string, flags exportFlags) {
	workbook, err := openInput(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "csvx export: %v\n", err)
		os.Exit(1)
	}
	if !flags.all {
		text, warnings, err := csvx.ExportCSV(workbook, flags.CSVExportOptions)
		if err != nil {
			fmt.Fprintf(os.Stderr, "csvx export: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(output, []byte(text), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "csvx export: %v\n", err)
			os.Exit(1)
		}
		printExportWarnings(warnings)
		fmt.Printf("exported: %s\n", output)
		return
	}
	if flags.Sheet != "" {
		fmt.Fprintf(os.Stderr, "csvx export: --all and --sheet cannot be combined\n")
		os.Exit(2)
	}
	if err := os.MkdirAll(output, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "csvx export: %v\n", err)
		os.Exit(1)
	}
	for i, name := range csvFileNames(workbook) {
		options := flags.CSVExportOptions
		options.Sheet = workbook.Sheets[i].ID
		text, warnings, err := csvx.ExportCSV(workbook, options)
		if err != nil {
			fmt.Fprintf(os.Stderr, "csvx export: sheet %q: %v\n", workbook.Sheets[i].Name, err)
			os.Exit(1)
		}
		target := filepath.Join(output, name)
		if err := os.WriteFile(target, []byte(text), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "csvx export: %v\n", err)
			os.Exit(1)
		}
		printExportWarnings(warnings)
		fmt.Printf("exported: %s\n", target)
	}
}

// isCSVImport reports whether the first positional argument (skipping flag values) is a .csv file. Plain CSV import is
// csvx-spec/spec/11-import-export.md §11.1; the CLI only parses flags and prints — the conversion
// itself is csvx-go's ImportCSVFile.
func isCSVImport(arguments []string) bool {
	for index := 0; index < len(arguments); index++ {
		switch argument := arguments[index]; {
		case argument == "--name" || argument == "--delimiter":
			index++ // skip the flag's value
		case !strings.HasPrefix(argument, "-"):
			return strings.ToLower(filepath.Ext(argument)) == ".csv"
		}
	}
	return false
}

func runImportCSV(arguments []string) {
	input, output, options, showHelp, err := parseImportCSVArguments(arguments)
	if err != nil {
		fmt.Fprintf(os.Stderr, "csvx import: %v\n\n", err)
		printCommandHelp(os.Stderr, "import")
		os.Exit(2)
	}
	if showHelp {
		printCommandHelp(os.Stdout, "import")
		return
	}
	if ext := strings.ToLower(filepath.Ext(output)); ext != ".csvx" {
		fmt.Fprintf(os.Stderr, "csvx import: expected a .csvx output, got %q\n", output)
		os.Exit(2)
	}
	workbook, warnings, err := csvx.ImportCSVFile(input, options)
	if err != nil {
		fmt.Fprintf(os.Stderr, "csvx import: %v\n", err)
		os.Exit(1)
	}
	if err := csvx.WritePackage(workbook, output); err != nil {
		fmt.Fprintf(os.Stderr, "csvx import: %v\n", err)
		os.Exit(1)
	}
	for _, warning := range warnings {
		fmt.Fprintf(os.Stderr, "warning: %s: %s\n", warning.Location, warning.Reason)
	}
	fmt.Printf("imported: %s\n", output)
}

func parseImportCSVArguments(arguments []string) (input, output string, options csvx.CSVImportOptions, help bool, err error) {
	var positional []string
	for index := 0; index < len(arguments); index++ {
		switch argument := arguments[index]; argument {
		case "--help", "-h":
			return "", "", options, true, nil
		case "--no-header":
			options.NoHeader = true
		case "--infer":
			options.Infer = true
		case "--name", "--delimiter":
			if index+1 >= len(arguments) {
				return "", "", options, false, fmt.Errorf("%s requires a value", argument)
			}
			index++
			if argument == "--name" {
				options.Name = arguments[index]
				break
			}
			delimiter := arguments[index]
			if delimiter == "tab" || delimiter == "\\t" {
				delimiter = "\t"
			}
			if utf8.RuneCountInString(delimiter) != 1 {
				return "", "", options, false, fmt.Errorf("--delimiter must be a single character (or \"tab\"), got %q", arguments[index])
			}
			options.Delimiter, _ = utf8.DecodeRuneInString(delimiter)
		default:
			if strings.HasPrefix(argument, "-") {
				return "", "", options, false, fmt.Errorf("unknown flag %q", argument)
			}
			positional = append(positional, argument)
		}
	}
	if len(positional) != 2 {
		return "", "", options, false, fmt.Errorf("provide an input .csv file and an output .csvx file")
	}
	return positional[0], positional[1], options, false, nil
}
