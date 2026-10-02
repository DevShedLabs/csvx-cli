package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime/debug"

	csvx "github.com/DevShedLabs/csvx-go"
)

// version is read from the module's own build info rather than hardcoded, so it can't drift from
// the git tag the way a literal string constant just did: v0.1.1 was tagged on a commit that still
// said "0.1.0-dev". `go install .../csvx@vX.Y.Z` and `go build` both populate this; a plain `go run`
// or a build from a checkout with no VCS info does not, hence the fallback.
func version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return "dev"
	}
	return info.Main.Version
}

func main() {
	if len(os.Args) < 2 {
		printHelp(os.Stderr)
		os.Exit(2)
	}

	if isHelp(os.Args[1]) {
		printHelp(os.Stdout)
		return
	}
	if isVersion(os.Args[1]) {
		fmt.Printf("csvx %s\n", version())
		return
	}

	command := os.Args[1]
	knownCommands := map[string]bool{
		"inspect": true, "validate": true, "package": true, "extract": true,
		"xlsx-inspect": true, "convert": true, "create": true, "import": true,
		"export": true, "codegen": true, "gen": true, "tags": true, "update": true,
		"self-update": true,
	}
	if !knownCommands[command] {
		fmt.Fprintf(os.Stderr, "csvx: unknown command %q\n\n", command)
		printHelp(os.Stderr)
		os.Exit(2)
	}

	if command == "package" {
		runPackage(os.Args[2:])
		return
	}
	if command == "extract" {
		runExtract(os.Args[2:])
		return
	}
	if command == "xlsx-inspect" {
		runXLSXInspect(os.Args[2:])
		return
	}
	if command == "convert" {
		runConvert(os.Args[2:])
		return
	}
	if command == "create" {
		runCreate(os.Args[2:])
		return
	}
	if command == "import" {
		runImport(os.Args[2:])
		return
	}
	if command == "export" {
		runExport(os.Args[2:])
		return
	}
	if command == "codegen" {
		runCodegen(os.Args[2:])
		return
	}
	if command == "gen" {
		runGen(os.Args[2:])
		return
	}
	if command == "tags" {
		runTags(os.Args[2:])
		return
	}
	if command == "update" {
		runUpdate(os.Args[2:])
		return
	}
	if command == "self-update" {
		runSelfUpdate(os.Args[2:])
		return
	}

	filename, help, jsonOutput, err := parseInputArguments(os.Args[2:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "csvx %s: %v\n\n", command, err)
		printCommandHelp(os.Stderr, command)
		os.Exit(2)
	}
	if help {
		printCommandHelp(os.Stdout, command)
		return
	}

	workbook, err := openInput(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "csvx %s: %v\n", command, err)
		os.Exit(1)
	}

	switch command {
	case "inspect":
		if err := printJSON(workbook); err != nil {
			fmt.Fprintf(os.Stderr, "csvx inspect: %v\n", err)
			os.Exit(1)
		}
	case "validate":
		result := csvx.Validate(filename)
		if jsonOutput {
			if err := printJSON(result); err != nil {
				fmt.Fprintf(os.Stderr, "csvx validate: %v\n", err)
				os.Exit(1)
			}
			if !result.Valid {
				os.Exit(1)
			}
			return
		}
		if !result.Valid {
			fmt.Fprintf(os.Stderr, "invalid: %s\n", filename)
			for _, diagnostic := range result.Errors {
				fmt.Fprintf(os.Stderr, "  [%s] %s\n", diagnostic.Code, diagnostic.Message)
			}
			os.Exit(1)
		}
		fmt.Printf("valid: %s\n", filename)
	}
}

func runPackage(arguments []string) {
	input, output, showHelp, err := parsePackageArguments(arguments)
	if err != nil {
		fmt.Fprintf(os.Stderr, "csvx package: %v\n\n", err)
		printCommandHelp(os.Stderr, "package")
		os.Exit(2)
	}
	if showHelp {
		printCommandHelp(os.Stdout, "package")
		return
	}
	if err := csvx.PackageDirectory(input, output); err != nil {
		fmt.Fprintf(os.Stderr, "csvx package: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("created: %s\n", output)
}

func parsePackageArguments(arguments []string) (string, string, bool, error) {
	var input, output string
	for index := 0; index < len(arguments); index++ {
		switch arguments[index] {
		case "--help", "-h":
			return "", "", true, nil
		case "--output", "-o":
			if index+1 >= len(arguments) {
				return "", "", false, fmt.Errorf("--output requires a file path")
			}
			output = arguments[index+1]
			index++
		default:
			if input != "" {
				return "", "", false, fmt.Errorf("expected one input directory, got %q", arguments[index])
			}
			input = arguments[index]
		}
	}
	if input == "" || output == "" {
		return "", "", false, fmt.Errorf("provide an input directory and --output file.csvx")
	}
	return input, output, false, nil
}

func runExtract(arguments []string) {
	input, output, showHelp, err := parseExtractArguments(arguments)
	if err != nil {
		fmt.Fprintf(os.Stderr, "csvx extract: %v\n\n", err)
		printCommandHelp(os.Stderr, "extract")
		os.Exit(2)
	}
	if showHelp {
		printCommandHelp(os.Stdout, "extract")
		return
	}
	if err := csvx.ExtractPackage(input, output); err != nil {
		fmt.Fprintf(os.Stderr, "csvx extract: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("extracted: %s\n", output)
}

func parseExtractArguments(arguments []string) (string, string, bool, error) {
	var input, output string
	for index := 0; index < len(arguments); index++ {
		switch arguments[index] {
		case "--help", "-h":
			return "", "", true, nil
		case "--output", "-o":
			if index+1 >= len(arguments) {
				return "", "", false, fmt.Errorf("--output requires a directory path")
			}
			output = arguments[index+1]
			index++
		default:
			if input != "" {
				return "", "", false, fmt.Errorf("expected one input package, got %q", arguments[index])
			}
			input = arguments[index]
		}
	}
	if input == "" || output == "" {
		return "", "", false, fmt.Errorf("provide an input .csvx file and --output directory")
	}
	return input, output, false, nil
}

func runConvert(arguments []string) {
	input, output, showHelp, err := parseConvertArguments(arguments)
	if err != nil {
		fmt.Fprintf(os.Stderr, "csvx convert: %v\n\n", err)
		printCommandHelp(os.Stderr, "convert")
		os.Exit(2)
	}
	if showHelp {
		printCommandHelp(os.Stdout, "convert")
		return
	}
	if err := csvx.Convert(input, output); err != nil {
		fmt.Fprintf(os.Stderr, "csvx convert: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("created: %s\n", output)
}

func parseConvertArguments(arguments []string) (string, string, bool, error) {
	if len(arguments) == 1 && (arguments[0] == "--help" || arguments[0] == "-h") {
		return "", "", true, nil
	}
	if len(arguments) != 2 {
		return "", "", false, fmt.Errorf("provide an input file and output file")
	}
	return arguments[0], arguments[1], false, nil
}

func runXLSXInspect(arguments []string) {
	filename, help, jsonOutput, err := parseInputArguments(arguments)
	if err != nil {
		fmt.Fprintf(os.Stderr, "csvx xlsx-inspect: %v\n\n", err)
		printCommandHelp(os.Stderr, "xlsx-inspect")
		os.Exit(2)
	}
	if help {
		printCommandHelp(os.Stdout, "xlsx-inspect")
		return
	}
	inspection, err := csvx.InspectXLSX(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "csvx xlsx-inspect: %v\n", err)
		os.Exit(1)
	}
	if jsonOutput {
		if err := printJSON(inspection); err != nil {
			fmt.Fprintf(os.Stderr, "csvx xlsx-inspect: %v\n", err)
			os.Exit(1)
		}
		return
	}
	fmt.Printf("XLSX: %s\nSHA-256: %s\nSheets: %d\nResources: %d\n", inspection.Filename, inspection.SHA256, len(inspection.Sheets), len(inspection.Resources))
	for _, warning := range inspection.Warnings {
		fmt.Printf("[%s] %s: %s\n", warning.Severity, warning.Feature, warning.Message)
	}
}

func openInput(filename string) (*csvx.Workbook, error) {
	info, err := os.Stat(filename)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return csvx.OpenDirectory(filename)
	}
	return csvx.Open(filename)
}

func isHelp(argument string) bool {
	return argument == "--help" || argument == "-h" || argument == "help"
}

// isVersion recognizes both the common CLI convention (--version / -v) and the original "version"
// subcommand, which is kept for backward compatibility.
func isVersion(argument string) bool {
	return argument == "--version" || argument == "-v" || argument == "version"
}

// parseInputArguments parses the shared `[flags] <input>` shape used by inspect and validate. It
// scans every argument for flags regardless of position, unlike Go's flag package, which stops
// recognizing flags as soon as it sees the first positional argument — meaning
// `csvx validate example.csvx --json` would otherwise fail while `csvx validate --json
// example.csvx` works, which is a real usability bug, not an acceptable CLI convention.
func parseInputArguments(arguments []string) (filename string, help bool, jsonOutput bool, err error) {
	for _, argument := range arguments {
		switch argument {
		case "--help", "-h":
			help = true
		case "--json":
			jsonOutput = true
		default:
			if filename != "" {
				return "", false, false, fmt.Errorf("expected exactly one input path, got %q", argument)
			}
			filename = argument
		}
	}
	if !help && filename == "" {
		return "", false, false, fmt.Errorf("expected exactly one input path")
	}
	return filename, help, jsonOutput, nil
}

func printJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func printHelp(output *os.File) {
	fmt.Fprintln(output, "CSVX spreadsheet package tools")
	fmt.Fprintln(output, "")
	fmt.Fprintln(output, "Usage:")
	fmt.Fprintln(output, "  csvx <command> [options] <input>")
	fmt.Fprintln(output, "")
	fmt.Fprintln(output, "Commands:")
	fmt.Fprintln(output, "  create     Scaffold a new, minimal, schema-valid CSVX package")
	fmt.Fprintln(output, "  import     Import an XLSX file as a new CSVX package")
	fmt.Fprintln(output, "  export     Export a CSVX package's embedded original to XLSX")
	fmt.Fprintln(output, "  inspect    Print the loaded workbook as JSON")
	fmt.Fprintln(output, "  validate   Load and validate a CSVX package")
	fmt.Fprintln(output, "  package    Package an unpacked directory into a .csvx ZIP file")
	fmt.Fprintln(output, "  extract    Extract a .csvx ZIP file into an unpacked directory")
	fmt.Fprintln(output, "  xlsx-inspect Inspect an XLSX package and report detected features")
	fmt.Fprintln(output, "  convert    Convert XLSX to CSVX or recover embedded XLSX source (generic; prefer import/export)")
	fmt.Fprintln(output, "  codegen    Regenerate an engine's data model from csvx-spec/schemas")
	fmt.Fprintln(output, "  gen test.csvx  Generate a schema-exhaustive CSVX fixture")
	fmt.Fprintln(output, "  tags       List recent tagged versions of an engine module (default: csvx-go)")
	fmt.Fprintln(output, "  update     Pin an engine module to a specific tagged version")
	fmt.Fprintln(output, "  self-update  Reinstall csvx itself at the latest (or a given) tagged version")
	fmt.Fprintln(output, "")
	fmt.Fprintln(output, "  --version, -v    Print the CLI version")
	fmt.Fprintln(output, "  --help, -h       Show this help")
	fmt.Fprintln(output, "")
	fmt.Fprintln(output, "Input may be a .csvx ZIP file or an unpacked CSVX directory.")
	fmt.Fprintln(output, "Flags may appear before or after the input path.")
	fmt.Fprintln(output, "Use 'csvx <command> --help' for command-specific help.")
}

func printCommandHelp(output *os.File, command string) {
	switch command {
	case "inspect":
		fmt.Fprintln(output, "Usage: csvx inspect [--help] <input>")
		fmt.Fprintln(output, "")
		fmt.Fprintln(output, "Loads a CSVX ZIP file or unpacked package directory and prints the workbook as JSON.")
	case "validate":
		fmt.Fprintln(output, "Usage: csvx validate [--help] <input>")
		fmt.Fprintln(output, "")
		fmt.Fprintln(output, "Loads a CSVX ZIP file or unpacked package directory and reports whether it is valid.")
		fmt.Fprintln(output, "Use --json for machine-readable diagnostics.")
	case "package":
		fmt.Fprintln(output, "Usage: csvx package [--help] --output <file.csvx> <directory>")
		fmt.Fprintln(output, "")
		fmt.Fprintln(output, "Packages an unpacked CSVX directory into a ZIP-based .csvx file.")
	case "extract":
		fmt.Fprintln(output, "Usage: csvx extract [--help] --output <directory> <file.csvx>")
		fmt.Fprintln(output, "")
		fmt.Fprintln(output, "Extracts a ZIP-based .csvx file into an unpacked directory for inspection or editing.")
	case "xlsx-inspect":
		fmt.Fprintln(output, "Usage: csvx xlsx-inspect [--json] <file.xlsx>")
		fmt.Fprintln(output, "")
		fmt.Fprintln(output, "Inspects an XLSX package without executing macros or external links.")
	case "convert":
		fmt.Fprintln(output, "Usage: csvx convert <input.xlsx|csvx> <output.csvx|xlsx>")
		fmt.Fprintln(output, "")
		fmt.Fprintln(output, "Imports XLSX with embedded source preservation, or recovers an unchanged embedded XLSX source.")
		fmt.Fprintln(output, "Prefer 'csvx import' / 'csvx export', which validate the expected direction explicitly.")
	case "create":
		fmt.Fprintln(output, "Usage: csvx create [--sheet <name>] <output>")
		fmt.Fprintln(output, "")
		fmt.Fprintln(output, "Scaffolds a new, minimal, schema-valid CSVX package at <output>: a directory if it has no")
		fmt.Fprintln(output, ".csvx extension, or a packaged .csvx ZIP file if it does.")
	case "import":
		fmt.Fprintln(output, "Usage: csvx import <input.xlsx> <output.csvx>")
		fmt.Fprintln(output, "")
		fmt.Fprintln(output, "Imports an XLSX file as a new CSVX package, preserving the original as embedded source.")
	case "export":
		fmt.Fprintln(output, "Usage: csvx export <input.csvx> <output.xlsx>")
		fmt.Fprintln(output, "")
		fmt.Fprintln(output, "Recovers the unmodified XLSX source embedded in a CSVX package produced by 'csvx import'.")
		fmt.Fprintln(output, "Exporting an edited/arbitrary CSVX workbook to XLSX is not implemented yet (open gap, see")
		fmt.Fprintln(output, "csvx-spec/AGENTS.md rule 4.5).")
	case "codegen":
		fmt.Fprintln(output, "Usage: csvx codegen --lang <go|ts> --schema-dir <path> --out <file> [--package <name>]")
		fmt.Fprintln(output, "")
		fmt.Fprintln(output, "Regenerates an engine's data model from csvx-spec/schemas/*.schema.json, mechanically,")
		fmt.Fprintln(output, "instead of hand-typing structs/interfaces that can drift from the schema (see")
		fmt.Fprintln(output, "csvx-spec/AGENTS.md rule 3.1).")
		fmt.Fprintln(output, "")
		fmt.Fprintln(output, "--lang go requires go-jsonschema on PATH (`go install github.com/atombender/go-jsonschema@latest`).")
		fmt.Fprintln(output, "--lang ts requires json2ts on PATH (`npm install -g json-schema-to-typescript`).")
		fmt.Fprintln(output, "")
		fmt.Fprintln(output, "Examples:")
		fmt.Fprintln(output, "  (from a csvx-go checkout)")
		fmt.Fprintln(output, "  csvx codegen --lang go --schema-dir ../csvx-spec/schemas --out internal/schema/generated.go")
		fmt.Fprintln(output, "  (from a csvx-ts checkout)")
		fmt.Fprintln(output, "  csvx codegen --lang ts --schema-dir ../csvx-spec/schemas --out src/schema/generated.ts")
	case "gen":
		fmt.Fprintln(output, "Usage: csvx gen test.csvx [--output <path>]")
		fmt.Fprintln(output, "")
		fmt.Fprintln(output, "Generates a schema-exhaustive CSVX fixture covering every scalar type, every core error")
		fmt.Fprintln(output, "code, every style property, multiple sheets, a representative formula set, and every")
		fmt.Fprintln(output, "validation rule type — derived from csvx-spec/schemas and spec/04-data-types.md, not from")
		fmt.Fprintln(output, "copying the small hand-authored examples/ fixtures (see csvx-spec/AGENTS.md rule 4.4).")
		fmt.Fprintln(output, "Output is a directory unless <path> ends in .csvx. Default output is ./test.csvx.")
	case "tags":
		fmt.Fprintln(output, "Usage: csvx tags [--module <path>]")
		fmt.Fprintln(output, "")
		fmt.Fprintln(output, "Lists the 5 most recent tagged versions of an engine module (default")
		fmt.Fprintln(output, "github.com/DevShedLabs/csvx-go), queried directly from its VCS host rather than through the")
		fmt.Fprintln(output, "Go module proxy's cache — a tag pushed minutes ago can otherwise look invisible until the")
		fmt.Fprintln(output, "proxy's cached version list expires. Run from within the Go module you want to update.")
	case "update":
		fmt.Fprintln(output, "Usage: csvx update [--module <path>] [version]")
		fmt.Fprintln(output, "")
		fmt.Fprintln(output, "Anywhere that is not a Go project depending on the engine, 'csvx update' (no version)")
		fmt.Fprintln(output, "updates the csvx binary itself, same as 'csvx self-update'. The rest of this help is for")
		fmt.Fprintln(output, "developers pinning the engine inside their own Go project.")
		fmt.Fprintln(output, "")
		fmt.Fprintln(output, "Pins an engine module (default github.com/DevShedLabs/csvx-go) to an exact tagged version —")
		fmt.Fprintln(output, "see 'csvx tags' to list recent ones. Runs 'go get <module>@<version>', then 'go mod tidy',")
		fmt.Fprintln(output, "then 'go build ./...' as a smoke check, all bypassing the module proxy cache the same way")
		fmt.Fprintln(output, "'csvx tags' does. Deliberately takes an exact version, never 'latest' — that's what makes")
		fmt.Fprintln(output, "stale-proxy-cache surprises go away: you see the real tag list first, then pin one exactly.")
		fmt.Fprintln(output, "In that mode a version is required and it must be run from within the Go module whose")
		fmt.Fprintln(output, "dependency you want to update.")
	case "self-update":
		fmt.Fprintln(output, "Usage: csvx self-update [version]")
		fmt.Fprintln(output, "")
		fmt.Fprintln(output, "Reinstalls the csvx binary itself via 'go install', bypassing the module proxy cache. With")
		fmt.Fprintln(output, "no argument, resolves the real latest csvx-cli tag directly from its VCS host first (never")
		fmt.Fprintln(output, "passing the literal '@latest' to go install, which is exactly what gets stuck on a stale")
		fmt.Fprintln(output, "cached resolution) and installs that exact version. Pass an exact tag (e.g. v0.1.3) to pin one.")
	}
}
