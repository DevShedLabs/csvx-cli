# Architecture rules for this repo

The binding rules for this project live in `../csvx-spec/AGENTS.md`. Read it before making any
change here. The short version for this repo specifically:

- `csvx-spec` is the authority. This engine implements the spec; it does not define it.
- Generate data model structs from `../csvx-spec/schemas/*.json`; do not hand-type them. A
  hand-typed struct (`Styles map[string]map[string]any` in `model.go`) is exactly what caused
  this engine's `styles.json` output to silently diverge from `styles.schema.json` — the schema
  wants an array of `{id, ...}` objects, not a numeric-keyed map.
- Validate this engine's actual JSON output against `../csvx-spec/schemas/*.json` in CI, on every
  change. That check never existed, which is how the bug above shipped unnoticed.
- Run `../csvx-spec/tests/*.json` conformance vectors through this engine in CI.
- If you need a field or behavior the spec doesn't describe, fix `csvx-spec` first — spec, schema,
  and test vector — then implement it here. Don't invent it here and let the spec catch up later.

See `../csvx-spec/AGENTS.md` for full detail and the reasoning behind these rules.
