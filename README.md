# CSVX CLI

`csvx-cli` is the one dedicated command-line tool for the CSVX spreadsheet format — a single Go
binary. It calls into engine libraries (`csvx-go` today) for the actual load/edit/calculate/write
work rather than reimplementing it; see `AGENTS.md` and `../csvx-spec/AGENTS.md` for why that split
exists and what belongs where.

## Install

```bash
go install github.com/DevShedLabs/csvx-cli/cmd/csvx@latest
```

This installs a `csvx` binary to `$(go env GOPATH)/bin` (make sure that's on your `PATH`). Requires
Go 1.22+.

This module has no version tags yet, so `@latest` resolves against `proxy.golang.org`'s cache of
the default branch, which can lag a few minutes behind a fresh push. If `go install` fails right
after a push (especially with a `replace directives` error referencing an old commit), that's
proxy staleness, not a real problem — either wait a few minutes and retry, or bypass the proxy:

```bash
GOPROXY=direct GOSUMDB=off go install github.com/DevShedLabs/csvx-cli/cmd/csvx@latest
```

or pin the exact commit to skip `@latest` resolution entirely:

```bash
go install github.com/DevShedLabs/csvx-cli/cmd/csvx@<commit-sha>
```

### From source

```bash
git clone https://github.com/DevShedLabs/csvx-cli.git
cd csvx-cli
go build -o bin/csvx ./cmd/csvx
./bin/csvx --help
```

## Usage

Both `.csvx` ZIP files and unpacked CSVX package directories are accepted as input.

```bash
csvx --help
csvx version

# Convert a real XLSX workbook to CSVX (embeds the original for lossless recovery)
csvx convert report.xlsx report.csvx

# Recover an unmodified embedded XLSX source from a .csvx package
csvx convert report.csvx report.xlsx

# Inspect a package
csvx inspect report.csvx
csvx inspect report.csvx --json    # not yet supported on inspect; use validate --json below

# Validate a package (structural load-based check; see "Validation" below)
csvx validate report.csvx
csvx validate --json report.csvx

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

- `inspect`, `validate`, `package`, `extract`, `xlsx-inspect`, `convert`, `version`
- Planned: `create`, `export`/`import` as first-class names (see `handoff.md`), `codegen`, and
  `gen test.csvx` — see `AGENTS.md` for what each is responsible for.

Formula parsing and recalculation are not implemented yet (that's `csvx-go`'s scope, not this
repo's — see its own README/handoff).
