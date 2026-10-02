# CSVX CLI

`csvx-cli` is the one dedicated command-line tool for the CSVX spreadsheet format — a single Go
binary. It calls into engine libraries (`csvx-go` today) for the actual load/edit/calculate/write
work rather than reimplementing it; see `AGENTS.md` and `../csvx-spec/AGENTS.md` for why that split
exists and what belongs where.

## Internal 
> This project uses the CSVX Spec and Go engine

- [CSVX Spec](https://github.com/DevShedLabs/csvx-spec)
- [Go Engine](https://github.com/DevShedLabs/csvx-go)

## Install

```bash
go install github.com/DevShedLabs/csvx-cli/cmd/csvx@v0.1.11
```

This installs a `csvx` binary to `$(go env GOPATH)/bin` (make sure that's on your `PATH`). Requires
Go 1.22+. Pin to the tag (`@v0.1.0`), not `@latest` — a tag is an exact, unambiguous reference;
`@latest` against a module's default branch is resolved by `proxy.golang.org` and can lag behind a
fresh push by a few minutes before a new tag exists to pin to instead. If `go install` ever fails
right after a release with a `replace directives` error, that's proxy staleness — bypass it with:

```bash
GOPROXY=direct GOSUMDB=off go install github.com/DevShedLabs/csvx-cli/cmd/csvx@latest
```

Once you have any version installed, `csvx self-update` automates exactly this workaround for you
going forward — see "Updating" below.

## Updating

Three commands manage version pins, and all three deliberately resolve versions *directly against
GitHub* rather than through `proxy.golang.org` — the public Go module proxy caches both "what
versions exist" and module contents for a meaningful TTL, so a tag pushed minutes ago can be
invisible to a plain `go get`/`go install` (including `@latest`) until that cache expires. None of
these three ever pass the literal string `@latest` to `go get`/`go install` for exactly that reason;
`tags` resolves the real version list first, then the others pin an exact, unambiguous tag.

```bash
# List the 5 most recent tags of an engine module (default: csvx-go)
csvx tags
csvx tags --module github.com/DevShedLabs/csvx-cli

# Pin a dependency in the current Go module's go.mod to an exact tag, then `go mod tidy` and
# `go build ./...` as a smoke check — run this from inside the project depending on the engine
csvx update v0.1.3
csvx update --module github.com/DevShedLabs/csvx-go v0.1.1

# Reinstall the csvx binary itself. With no argument, resolves the true latest csvx-cli tag
# directly (never the cached "@latest") and installs exactly that version; pass a tag to pin one.
csvx self-update
csvx self-update v0.1.1
```

`update` and `self-update` are different operations: `update` edits a dependency pin inside some
*other* Go module's `go.mod` (e.g. bumping `csvx-go` inside a project that imports it); `self-update`
replaces the `csvx` binary itself via `go install` and has no `go.mod` of its own to edit.

### From source

```bash
git clone https://github.com/DevShedLabs/csvx-cli.git
cd csvx-cli
go build -o bin/csvx ./cmd/csvx
./bin/csvx --help
```

## Usage

Both `.csvx` ZIP files and unpacked CSVX package directories are accepted as input. Flags may
appear before or after the input path (`csvx validate report.csvx --json` and
`csvx validate --json report.csvx` are equivalent) — this wasn't always true and is worth relying
on explicitly, not just something that happens to work.

```bash
csvx --help
csvx --version    # or: csvx -v, or the original csvx version (all three work)

# Convert a real XLSX workbook to CSVX (embeds the original for lossless recovery)
csvx convert report.xlsx report.csvx

# Recover an unmodified embedded XLSX source from a .csvx package
csvx convert report.csvx report.xlsx

# Inspect a package (always prints JSON; there's no non-JSON mode for inspect)
csvx inspect report.csvx

# Validate a package (structural load-based check; see "Validation" below)
csvx validate report.csvx
csvx validate --json report.csvx
csvx validate report.csvx --json   # same result, flag order doesn't matter

# Round-trip a package as a directory for manual editing
csvx extract report.csvx --output report-unpacked
# ... edit CSV/.meta.json files by hand ...
csvx package report-unpacked --output report-edited.csvx

# Inspect an XLSX file before converting it (no macros or external links are ever executed)
csvx xlsx-inspect report.xlsx
csvx xlsx-inspect --json report.xlsx
```

### Validation

`csvx validate` today performs a structural, load-based check — it confirms the package parses
(required entries present, CSV/JSON well-formed). It does **not** yet validate against
`../csvx-spec/schemas/*.json`. For real schema-conformance checking, use the reference validator
until `validate` grows native JSON Schema support (tracked in `handoff.md`):

```bash
cd ../csvx-spec/validator
npm install
node bin/csvx-validate.mjs path/to/report.csvx
```

## Development

```bash
go build ./...          # build every package
go vet ./...             # static analysis
go test ./...            # run all tests, including the real end-to-end CLI tests (see below)
go test -race ./...
gofmt -w .               # format before committing
```

### Pre-push checks (local, since GitHub Actions minutes are limited)

`scripts/check.sh` runs the same checks CI would (`gofmt`, `go vet`, `go build`, `go test` —
including the real end-to-end and schema-validation tests below). Run it any time:

```bash
./scripts/check.sh
```

A git hook runs it automatically before every push, blocking the push if it fails. Enable it once
per clone (this is local git config, not something that comes from cloning the repo):

```bash
git config core.hooksPath .githooks
```

Skip in a genuine emergency with `git push --no-verify` — prefer fixing the failure instead. See
`../csvx-spec/AGENTS.md` section 6 for why this exists: it's the interim stand-in for real CI.

### Testing

Tests here run against real fixture files and the real compiled binary, not hand-invented minimal
data — see `../csvx-spec/AGENTS.md` rule 4.7 for why that's required, not optional. Concretely:

- `cmd/csvx/cli_e2e_test.go` builds the actual `csvx` binary and drives it as a subprocess with real
  arguments against a real XLSX fixture (`../csvx-spec/examples/example.xlsx`), checking actual
  stdout and exit codes through the full `convert → validate → inspect → extract → package →
  validate` chain.
- `cmd/csvx/interop_test.go` is the runner for `../csvx-spec/tests/interop/`: it converts that same
  real fixture and checks specific cell values/formulas/styles at known coordinates, *and* schema-
  validates the real output by shelling out to `../csvx-spec/validator` — both checks run as part
  of the same `go test`.

Both require a sibling `../csvx-spec` checkout (with `validator/`'s `npm install` already run for
the schema-validation half) to find their fixtures; they skip cleanly if that checkout isn't found.

## Dependency on csvx-go

This repo depends on `github.com/DevShedLabs/csvx-go` as an ordinary Go module dependency — not a
vendored copy, not a local `replace` directive. If you're developing both repos side by side and
want changes in a local `csvx-go` checkout picked up immediately, add your own temporary
`replace github.com/DevShedLabs/csvx-go => ../csvx-go` line — just don't commit it; a committed
local-path `replace` breaks `go install` for everyone else (this shipped once already; don't repeat
it).

## Current scope

- `inspect`, `validate`, `package`, `extract`, `xlsx-inspect`, `convert`
- `tags`, `update`, `self-update` — see "Updating" above
- `--version`/`-v` (standard convention; `version` subcommand kept for backward compatibility)
- Planned: `create`, `export`/`import` as first-class names (see `handoff.md`), `codegen`, and
  `gen test.csvx` — see `AGENTS.md` for what each is responsible for.

Formula parsing and recalculation are not implemented yet (that's `csvx-go`'s scope, not this
repo's — see its own README/handoff).
