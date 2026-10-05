#!/bin/sh
# Run the same checks CI would run. Usable two ways:
#   ./scripts/check.sh          — run on demand any time during development
#   .githooks/pre-push          — runs this automatically before every push (see that file)
#
# This exists because GitHub Actions minutes are constrained on a free account right now — this is
# the interim local enforcement standing in for real CI (see ../csvx-spec/AGENTS.md section 6).
#
# `go test ./...` here is doing real work, not just unit tests: cmd/csvx/interop_test.go and
# cli_e2e_test.go build the actual binary, convert a real XLSX fixture, and shell out to
# ../csvx-spec/validator to check the output against the real JSON Schemas. A passing run here
# means the CLI actually works end to end against real data, not just that it compiles.
set -e

cd "$(dirname "$0")/.."

# GUI git clients (and some non-interactive hook environments) launch hooks with a bare-bones PATH
# that doesn't include whatever your shell profile adds — so `go`/`gofmt`/`node` can be "not found"
# here even though they work fine from a terminal. Extend PATH with common install locations rather
# than trusting the inherited environment. Note: this can't reliably find an nvm-managed `node`
# (its path is version-specific, e.g. ~/.nvm/versions/node/vX.Y.Z/bin) — if the schema-validation
# tests below fail with "node not found", either symlink/install node somewhere on this list, or run
# `nvm use` in the shell you push from so PATH already has it before the hook inherits it.
for dir in /usr/local/go/bin /opt/homebrew/bin /opt/homebrew/opt/go/bin "$HOME/go/bin" /usr/local/bin; do
	case ":$PATH:" in
	*":$dir:"*) ;;
	*) PATH="$PATH:$dir" ;;
	esac
done
export PATH

if ! command -v go >/dev/null 2>&1; then
	echo "check: 'go' not found even after extending PATH — if Go lives somewhere unusual on this" >&2
	echo "check: machine, add that directory to the list in $(basename "$0")" >&2
	exit 1
fi

echo "check: gofmt..."
unformatted=$(gofmt -l .)
if [ -n "$unformatted" ]; then
	echo "check: the following files need gofmt:" >&2
	echo "$unformatted" >&2
	echo "check: run 'gofmt -w .' and try again" >&2
	exit 1
fi

echo "check: go vet ./..."
go vet ./...

echo "check: go build ./..."
go build ./...

echo "check: go test ./... (includes real end-to-end + schema-validation checks)"
go test ./...

echo "check: spec coverage (csvx-spec/tools/coverage.mjs)..."
if [ -f ../csvx-spec/tools/coverage.mjs ] && command -v node >/dev/null 2>&1; then
	node ../csvx-spec/tools/coverage.mjs
else
	echo "check: csvx-spec checkout or node not found - skipping the coverage check" >&2
fi

echo "check: all checks passed"
