package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
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
// --lang go and --lang ts are implemented, covering csvx-go and csvx-ts, the two engines that
// exist today. A future language gets the same treatment: one function here, driving whatever
// mechanical generator is canonical for that language, never a hand-typed model.

type codegenOptions struct {
	lang      string
	schemaDir string
	out       string
	pkg       string
	help      bool
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
	case "ts":
		if err := codegenTS(options); err != nil {
			fmt.Fprintf(os.Stderr, "csvx codegen: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "csvx codegen: --lang %q is not implemented yet (only \"go\" and \"ts\" are); add it rather than hand-typing a model for that language\n", options.lang)
		os.Exit(1)
	}
	fmt.Printf("generated: %s\n", options.out)
}

func parseCodegenArguments(arguments []string) (codegenOptions, error) {
	options := codegenOptions{lang: "go", pkg: "schema"}
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
		return options, fmt.Errorf("provide --out <file>")
	}
	return options, nil
}

// codegenGo regenerates a Go data model from every schemas/*.schema.json file using go-jsonschema,
// the tool AGENTS.md rule 3.1 names and the one that already produced csvx-go's current
// internal/schema/generated.go. Running `csvx codegen --lang go --schema-dir ../csvx-spec/schemas
// --out internal/schema/generated.go` from csvx-go reproduces that file byte-for-byte, which is the
// point: schema changes regenerate the model instead of someone hand-editing structs to match.
func codegenGo(options codegenOptions) error {
	const schemaTool = "go-jsonschema"
	tool, err := exec.LookPath(schemaTool)
	if err != nil {
		return fmt.Errorf("%s not found on PATH; install it with `go install github.com/atombender/go-jsonschema@latest`: %w", schemaTool, err)
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

// codegenTS regenerates a TypeScript data model from every schemas/*.schema.json file using
// json-schema-to-typescript (its CLI, json2ts), the tool csvx-ts's own AGENTS.md names for this —
// so csvx-ts (and through it, eventually csvx-web, per csvx-spec/AGENTS.md rule 5) gets a real
// generated model instead of hand-typed interfaces that can drift from the schema the way
// csvx-go's styles.json struct once did.
//
// json2ts processes one input file at a time and, when a schema $refs a type defined in another
// file (e.g. sheet-metadata.schema.json's cell.cached references formulas.schema.json's Value),
// it inlines a full copy of that type into each file that references it rather than emitting a
// cross-file import. Run across all five schemas and simply concatenated, that would produce
// duplicate `export type Id = ...` / `export interface Value {...}` declarations — a TypeScript
// compile error. codegenTS collects every top-level declaration from every schema and keeps only
// one copy of each exact, byte-identical duplicate; since the duplicates come from the same source
// schema re-emitted by the same deterministic tool, an exact match is expected, and anything that
// comes back merely similar-but-different would mean the schemas disagree about that shared type —
// which is worth failing loudly on rather than silently picking one, so non-identical collisions
// are left as separate declarations and will fail to compile (surfacing the inconsistency) instead
// of being silently resolved.
func codegenTS(options codegenOptions) error {
	const schemaTool = "json2ts"
	tool, err := exec.LookPath(schemaTool)
	if err != nil {
		return fmt.Errorf("%s not found on PATH; install it with `npm install -g json-schema-to-typescript`: %w", schemaTool, err)
	}

	schemaFiles, err := schemaFilesIn(options.schemaDir)
	if err != nil {
		return err
	}
	if len(schemaFiles) == 0 {
		return fmt.Errorf("no *.schema.json files found in %s", options.schemaDir)
	}

	stagingDir, err := os.MkdirTemp("", "csvx-codegen-ts-*")
	if err != nil {
		return fmt.Errorf("create staging directory: %w", err)
	}
	defer os.RemoveAll(stagingDir)

	var declarations []string
	seen := make(map[string]bool)
	for _, schemaFile := range schemaFiles {
		generatedFile := filepath.Join(stagingDir, filepath.Base(schemaFile)+".ts")
		command := exec.Command(tool, "-i", schemaFile, "-o", generatedFile, "--cwd", options.schemaDir)
		if output, err := command.CombinedOutput(); err != nil {
			return fmt.Errorf("json2ts failed on %s: %w\n%s", schemaFile, err, output)
		}
		contents, err := os.ReadFile(generatedFile)
		if err != nil {
			return fmt.Errorf("read json2ts output for %s: %w", schemaFile, err)
		}
		for _, declaration := range splitTopLevelDeclarations(string(contents)) {
			key := strings.TrimSpace(declaration)
			if seen[key] {
				continue
			}
			seen[key] = true
			declarations = append(declarations, key)
		}
	}

	if err := os.MkdirAll(filepath.Dir(options.out), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	var combined bytes.Buffer
	combined.WriteString("/* eslint-disable */\n")
	combined.WriteString("/**\n")
	combined.WriteString(" * Generated by `csvx codegen --lang ts` from csvx-spec/schemas/*.schema.json.\n")
	combined.WriteString(" * DO NOT EDIT BY HAND. Modify the schema and regenerate instead — see\n")
	combined.WriteString(" * csvx-spec/AGENTS.md rule 3.1 for why this file must never be hand-typed.\n")
	combined.WriteString(" */\n\n")
	for index, declaration := range declarations {
		if index > 0 {
			combined.WriteString("\n")
		}
		combined.WriteString(declaration)
		combined.WriteString("\n")
	}

	return os.WriteFile(options.out, combined.Bytes(), 0o644)
}

// splitTopLevelDeclarations splits a json2ts output file (its license header already present) into
// its individual top-level `export ...` declarations. json2ts does not reliably separate adjacent
// declarations with a blank line (e.g. two one-line `export type` aliases in a row), so a blank-line
// split is not safe; instead, a new declaration starts at any line beginning with "export " and
// continues until the next such line or end of file.
func splitTopLevelDeclarations(source string) []string {
	scanner := bufio.NewScanner(strings.NewReader(source))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var declarations []string
	var current strings.Builder
	inDeclaration := false
	flush := func() {
		if inDeclaration {
			declarations = append(declarations, strings.TrimRight(current.String(), "\n"))
			current.Reset()
		}
	}
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "export ") {
			flush()
			inDeclaration = true
		}
		if inDeclaration {
			current.WriteString(line)
			current.WriteString("\n")
		}
	}
	flush()
	return declarations
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
