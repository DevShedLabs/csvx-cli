# Architecture rules for this repo

The binding rules for this project live in `../csvx-spec/AGENTS.md`. Read it before making any
change here. The short version for this repo specifically:

## What this repo is

`csvx-cli` is **the one dedicated CSVX command-line tool** — a single Go binary providing
`export`, `import`, `create`, `validate`, `codegen`, and `gen test.csvx` (a schema-exhaustive
fixture generator: every scalar type, multi-sheet, formulas, styles, validation rules). It exists
so there is exactly one place these operations live, instead of every language's engine growing
its own copy of the same CLI logic.

This repo was split out of `csvx-go` on 2026-09-30, which had grown both a library *and* a CLI in
one repo. `csvx-go` kept the library; this repo kept (and will build out) the CLI, importing
`csvx-go` as an ordinary Go module dependency rather than vendoring a private copy of its source.
A `replace github.com/DevShedLabs/csvx-go => ../csvx-go` directive in `go.mod` points at the local
sibling checkout during development — swap that for a real pinned version once `csvx-go` has
tagged releases.

## Rules specific to this repo

- **This repo does not duplicate engine logic.** `cmd/csvx` calls into `csvx-go` (and, later,
  bindings or subprocess calls to other language engines where relevant) for load/edit/
  calculate/write. If an operation needs new engine capability, that capability belongs in the
  engine repo, not reimplemented here.
- **`codegen` is how programmable interfaces get produced.** Each language's engine repo
  (`csvx-go`, `csvx-ts`, future Rust/Python/PHP) should end up mostly generated from
  `../csvx-spec/schemas/*.json` via this CLI's `codegen` command, not hand-typed. `csvx-go`'s
  `internal/schema/generated.go` (via `go-jsonschema`) is the first instance of this; folding that
  generation step into `csvx-cli codegen --lang go` (and adding `--lang ts`, etc.) is expected
  follow-up work, not optional polish.
- **`validate` is the one canonical schema validator, not a per-language reimplementation.** The
  reference implementation right now is the Node/ajv validator in `../csvx-spec/validator`
  (used during this repo's own bootstrapping and in CI). The end goal is a native Go JSON-Schema
  validator here so the CLI is a true single-binary install with no Node runtime dependency — but
  until that lands, don't let any other engine (`csvx-go`, `csvx-ts`, ...) grow its own native
  schema-validation logic. There is one validator; engines call it, they don't each reimplement it.
- **`gen test.csvx` must stay schema-derived, not example-derived.** It should walk
  `../csvx-spec/schemas/*.json` (and the scalar-type list in `spec/04-data-types.md`) to guarantee
  coverage of every type, style property, and structural feature — not just copy patterns from the
  existing hand-authored `examples/`, which are illustrative, not exhaustive.

See `../csvx-spec/AGENTS.md` for full detail and the reasoning behind these rules.
