package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// defaultUpdateModule is the one Go engine this project has today. --module overrides it, since
// nothing here is specific to csvx-go beyond this default — it's just the common case.
const defaultUpdateModule = "github.com/DevShedLabs/csvx-go"

// cliModule/cliPackage are csvx-cli's own module and installable package path — what self-update
// reinstalls, as distinct from defaultUpdateModule (a dependency some other project pins).
const cliModule = "github.com/DevShedLabs/csvx-cli"
const cliPackage = cliModule + "/cmd/csvx"

// goListVersions is the JSON shape `go list -m -json -versions <module>` prints.
type goListVersions struct {
	Path     string   `json:"Path"`
	Versions []string `json:"Versions"`
}

// withDirectProxy forces GOPROXY=direct for a command, bypassing the module proxy's cached
// version-list and module responses. The public Go module proxy (proxy.golang.org, the default)
// caches both "what versions exist" and the module contents themselves for a meaningful TTL, so a
// tag pushed minutes ago can look invisible to `go list`/`go get`/`go install` even though it's
// live on GitHub — exactly the staleness this project has hit before, both for a library
// dependency pin and for `go install .../csvx@latest` installing a stale binary. Going direct to
// the VCS host sidesteps that; it's slower (a real git fetch instead of a cached proxy hit) but
// correct, which is what `tags`, `update`, and `self-update` all need to be.
//
// Any existing GOPROXY already in the environment is filtered out first, not just appended past —
// if a shell sets GOPROXY explicitly, appending a second GOPROXY=direct after it is not guaranteed
// to win (environment variable precedence when a key is duplicated is implementation-defined), so
// this builds an environment with exactly one GOPROXY entry instead of trusting append order.
func withDirectProxy(cmd *exec.Cmd) {
	env := os.Environ()
	filtered := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if !strings.HasPrefix(entry, "GOPROXY=") {
			filtered = append(filtered, entry)
		}
	}
	cmd.Env = append(filtered, "GOPROXY=direct")
}

func runTags(arguments []string) {
	module, limit, help, err := parseTagsArguments(arguments)
	if err != nil {
		fmt.Fprintf(os.Stderr, "csvx tags: %v\n\n", err)
		printCommandHelp(os.Stderr, "tags")
		os.Exit(2)
	}
	if help {
		printCommandHelp(os.Stdout, "tags")
		return
	}

	versions, err := listModuleVersions(module)
	if err != nil {
		fmt.Fprintf(os.Stderr, "csvx tags: %v\n", err)
		os.Exit(1)
	}
	if len(versions) == 0 {
		fmt.Printf("%s: no tagged versions found\n", module)
		return
	}

	recent := versions
	if len(recent) > limit {
		recent = recent[len(recent)-limit:]
	}
	fmt.Printf("%s — most recent %d of %d tagged version(s):\n", module, len(recent), len(versions))
	for i := len(recent) - 1; i >= 0; i-- {
		marker := ""
		if i == len(recent)-1 {
			marker = "  (latest)"
		}
		fmt.Printf("  %s%s\n", recent[i], marker)
	}
	fmt.Println()
	fmt.Printf("Run 'csvx update <version>' (e.g. csvx update %s) to pin one.\n", recent[len(recent)-1])
}

func parseTagsArguments(arguments []string) (module string, limit int, help bool, err error) {
	module = defaultUpdateModule
	limit = 5
	for index := 0; index < len(arguments); index++ {
		switch arguments[index] {
		case "--help", "-h":
			return "", 0, true, nil
		case "--module":
			if index+1 >= len(arguments) {
				return "", 0, false, fmt.Errorf("--module requires a Go module path")
			}
			module = arguments[index+1]
			index++
		default:
			return "", 0, false, fmt.Errorf("unexpected argument %q", arguments[index])
		}
	}
	return module, limit, false, nil
}

// listModuleVersions returns every tagged version of module known to the VCS, oldest first (the
// same order `go list -m -versions` itself prints them in — Go already sorts these by semver).
func listModuleVersions(module string) ([]string, error) {
	cmd := exec.Command("go", "list", "-m", "-json", "-versions", module)
	withDirectProxy(cmd)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			return nil, fmt.Errorf("go list failed: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("go list failed: %w", err)
	}
	var result goListVersions
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("parsing go list output: %w", err)
	}
	return result.Versions, nil
}

// inGoProjectDependingOn reports whether the current directory is inside a Go module other than
// `module` itself — i.e. one where `go get module@version` can actually pin it. Outside any module,
// `go list -m` still succeeds (reporting "command-line-arguments"), so GOMOD is checked first.
func inGoProjectDependingOn(module string) bool {
	gomod, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil || strings.TrimSpace(string(gomod)) == "" || strings.TrimSpace(string(gomod)) == os.DevNull {
		return false
	}
	mainModule, err := exec.Command("go", "list", "-m").Output()
	return err == nil && strings.TrimSpace(string(mainModule)) != module
}

func runUpdate(arguments []string) {
	module, version, help, err := parseUpdateArguments(arguments)
	if err != nil {
		fmt.Fprintf(os.Stderr, "csvx update: %v\n\n", err)
		printCommandHelp(os.Stderr, "update")
		os.Exit(2)
	}
	if help {
		printCommandHelp(os.Stdout, "update")
		return
	}

	// `update` pins a dependency in the *current* project's go.mod, which only makes sense inside a
	// Go project that depends on the module (developers embedding the engine). Everywhere else —
	// no go.mod, or inside the engine module itself, where Go reports "is in the main module" — the
	// user almost certainly wants the csvx binary updated, so do that. Versions are not
	// interchangeable (engine tags vs csvx-cli tags), so a version given here is not reused.
	if !inGoProjectDependingOn(module) {
		if version != "" {
			fmt.Fprintf(os.Stderr, "csvx update: there is no Go project here that depends on %s, so %s can't be pinned.\n", module, version)
			fmt.Fprintln(os.Stderr, "That version is an engine version; csvx-cli has its own. To update the csvx binary, run")
			fmt.Fprintln(os.Stderr, "'csvx update' with no version (latest), or 'csvx self-update <csvx-cli version>'.")
			os.Exit(1)
		}
		fmt.Println("No Go project depends on the engine here, so updating the csvx binary itself.")
		runSelfUpdate(nil)
		return
	}
	if version == "" {
		fmt.Fprintln(os.Stderr, "csvx update: expected an engine version — run 'csvx tags' to see recent ones")
		os.Exit(2)
	}

	target := module + "@" + version
	fmt.Printf("go get %s (direct, bypassing the module proxy cache)...\n", target)
	getCmd := exec.Command("go", "get", target)
	withDirectProxy(getCmd)
	if output, err := getCmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "csvx update: go get failed:\n%s\n", output)
		os.Exit(1)
	}

	fmt.Println("go mod tidy...")
	tidyCmd := exec.Command("go", "mod", "tidy")
	withDirectProxy(tidyCmd)
	if output, err := tidyCmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "csvx update: go mod tidy failed:\n%s\n", output)
		os.Exit(1)
	}

	fmt.Println("go build ./... (smoke check)...")
	buildCmd := exec.Command("go", "build", "./...")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "csvx update: pinned %s, but the build broke:\n%s\n", target, output)
		fmt.Fprintln(os.Stderr, "go.mod/go.sum were still updated — fix the break or run 'csvx update <previous version>' to revert.")
		os.Exit(1)
	}

	fmt.Printf("updated: %s, and the build still passes.\n", target)
}

// runSelfUpdate reinstalls the csvx binary itself — a `go install` of a command, not a `go get` of
// a library dependency, so `update` (which edits some *other* project's go.mod) doesn't apply here.
// With no version given, it resolves the real latest tag itself via listModuleVersions (direct,
// not cached) and installs that exact resolved string — it never passes the literal "@latest" to
// `go install`, since that's precisely the path that gets stuck on a stale cached resolution.
func runSelfUpdate(arguments []string) {
	requested, help, err := parseSelfUpdateArguments(arguments)
	if err != nil {
		fmt.Fprintf(os.Stderr, "csvx self-update: %v\n\n", err)
		printCommandHelp(os.Stderr, "self-update")
		os.Exit(2)
	}
	if help {
		printCommandHelp(os.Stdout, "self-update")
		return
	}

	version := requested
	if version == "" {
		fmt.Println("Resolving the latest csvx-cli tag directly from its VCS host (not the module proxy cache)...")
		versions, err := listModuleVersions(cliModule)
		if err != nil {
			fmt.Fprintf(os.Stderr, "csvx self-update: %v\n", err)
			os.Exit(1)
		}
		if len(versions) == 0 {
			fmt.Fprintln(os.Stderr, "csvx self-update: no tagged versions found")
			os.Exit(1)
		}
		version = versions[len(versions)-1]
	}

	target := cliPackage + "@" + version
	fmt.Printf("go install %s (direct)...\n", target)
	cmd := exec.Command("go", "install", target)
	withDirectProxy(cmd)
	if output, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "csvx self-update: go install failed:\n%s\n", output)
		os.Exit(1)
	}

	fmt.Printf("installed csvx %s.\n", version)
	fmt.Println("If 'csvx --version' still shows the old version, something earlier on PATH is shadowing")
	fmt.Println("$(go env GOPATH)/bin/csvx — check 'which -a csvx' and PATH order.")
}

func parseSelfUpdateArguments(arguments []string) (version string, help bool, err error) {
	for index := 0; index < len(arguments); index++ {
		switch arguments[index] {
		case "--help", "-h":
			return "", true, nil
		default:
			if version != "" {
				return "", false, fmt.Errorf("expected at most one version, got %q", arguments[index])
			}
			version = arguments[index]
		}
	}
	if version != "" && !strings.HasPrefix(version, "v") {
		return "", false, fmt.Errorf("expected a semver tag like v0.1.3, got %q", version)
	}
	return version, false, nil
}

func parseUpdateArguments(arguments []string) (module string, version string, help bool, err error) {
	module = defaultUpdateModule
	for index := 0; index < len(arguments); index++ {
		switch arguments[index] {
		case "--help", "-h":
			return "", "", true, nil
		case "--module":
			if index+1 >= len(arguments) {
				return "", "", false, fmt.Errorf("--module requires a Go module path")
			}
			module = arguments[index+1]
			index++
		default:
			if version != "" {
				return "", "", false, fmt.Errorf("expected exactly one version, got %q", arguments[index])
			}
			version = arguments[index]
		}
	}
	if version != "" && !strings.HasPrefix(version, "v") {
		return "", "", false, fmt.Errorf("expected a semver tag like v0.1.3, got %q", version)
	}
	return module, version, false, nil
}
