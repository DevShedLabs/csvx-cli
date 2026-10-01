package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
)

// codegen is the mechanism described in csvx-spec/AGENTS.md rule 3.1: every engine's data model
// (Manifest, Workbook, Sheet, Style, cell metadata) must be generated mechanically from
// schemas/*.json, not hand-typed. Before this command existed, that generation step was a human
// running go-jsonschema by hand once (csvx-go's internal/schema/generated.go, 2026-09-30) with no
// recorded, repeatable invocation — exactly the kind of unenforced step that let the pre-2026-09-30
// styles.json bug happen in the first place. This command is that invocation, tracked and rerunnable
// any time schemas/ changes (AGENTS.md rule 4.3: regenerating every engine's model is part of
// landing a schema change, not a follow-up someone gets to later).
//
// Only --lang go is implemented today, matching csvx-go, the one mature engine. A future --lang ts
// (for csvx-ts, via quicktype or json-schema-to-typescript) is a real but separate addition — this
// command errors clearly on unsupported languages rather than silently doing nothing.

type codegenOptions struct {
	lang       string
	schemaDir  string
	out        string
	pkg        string
	schemaTool string
	help       bool
}

func runCodegen(arguments []string) {
	options, err := parseCodegenArguments(arguments)
	if err != nil {
		fmt.Fprintf(os.Stderr, "csvx codegen: %v\n\n", err)
		printCommandHelp(os.Stderr, "codegen")
		os.Exit(2)
	}
	if options.help {
		printCommandHelp(os.Stdout, "codegen")
		return
	}

	switch options.lang {
	case "go":
		if err := codegenGo(options); err != nil {
			fmt.Fprintf(os.Stderr, "csvx codegen: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "csvx codegen: --lang %q is not implemented yet (only \"go\" is); add it rather than hand-typing a model for that language\n", options.lang)
		os.Exit(1)
	}
	fmt.Printf("generated: %s\n", options.out)
}

func parseCodegenArguments(arguments []string) (codegenOptions, error) {
	options := codegenOptions{lang: "go", pkg: "schema", schemaTool: "go-jsonschema"}
	for index := 0; index < len(arguments); index++ {
		switch arguments[index] {
		case "--help", "-h":
			options.help = true
			return options, nil
		case "--lang":
			if index+1 >= len(arguments) {
				return options, fmt.Errorf("--lang requires a value (e.g. go)")
			}
			options.lang = arguments[index+1]
			index++
		case "--schema-dir":
			if index+1 >= len(arguments) {
				return options, fmt.Errorf("--schema-dir requires a path")
			}
			options.schemaDir = arguments[index+1]
			index++
		case "--out", "-o":
			if index+1 >= len(arguments) {
				return options, fmt.Errorf("--out requires a file path")
			}
			options.out = arguments[index+1]
			index++
		case "--package":
			if index+1 >= len(arguments) {
				return options, fmt.Errorf("--package requires a name")
			}
			options.pkg = arguments[index+1]
			index++
		default:
			return options, fmt.Errorf("unexpected argument %q", arguments[index])
		}
	}
	if options.schemaDir == "" {
		return options, fmt.Errorf("provide --schema-dir pointing at csvx-spec/schemas (no default is guessed, to avoid silently generating from the wrong checkout)")
	}
	if options.out == "" {
		return options, fmt.Errorf("provide --out <file.go>")
	}
	return options, nil
}

// codegenGo regenerates a Go data model from every schemas/*.schema.json file using go-jsonschema,
// the tool AGENTS.md rule 3.1 names and the one that already produced csvx-go's current
// internal/schema/generated.go. Running `csvx codegen --lang go --schema-dir ../csvx-spec/schemas
// --out internal/schema/generated.go` from csvx-go reproduces that file byte-for-byte, which is the
// point: schema changes regenerate the model instead of someone hand-editing structs to match.
func codegenGo(options codegenOptions) error {
	tool, err := exec.LookPath(options.schemaTool)
	if err != nil {
		return fmt.Errorf("%s not found on PATH; install it with `go install github.com/atombender/go-jsonschema@latest`: %w", options.schemaTool, err)
	}

	schemaFiles, err := schemaFilesIn(options.schemaDir)
	if err != nil {
		return err
	}
	if len(schemaFiles) == 0 {
		return fmt.Errorf("no *.schema.json files found in %s", options.schemaDir)
	}

	if err := os.MkdirAll(filepath.Dir(options.out), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	args := []string{"-p", options.pkg, "-t", "-o", options.out}
	args = append(args, schemaFiles...)
	command := exec.Command(tool, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go-jsonschema failed: %w\n%s", err, output)
	}
	return nil
}

// schemaFilesIn returns every *.schema.json file in dir, sorted, so generation is deterministic
// regardless of directory iteration order or which schemas were added when.
func schemaFilesIn(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read schema directory: %w", err)
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != ".json" {
			continue
		}
		files = append(files, filepath.Join(dir, name))
	}
	sort.Strings(files)
	return files, nil
}
