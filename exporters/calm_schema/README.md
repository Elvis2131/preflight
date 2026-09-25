# Vendored FINOS CALM schema (release 1.2)

`core.json`, `interface.json`, `control.json`, `flow.json` are verbatim copies of the
official FINOS CALM (Common Architecture Language Model) JSON Schema, fetched from
`github.com/finos/architecture-as-code`, path `calm/release/1.2/meta/`, commit
`afca8fd1f1fd3913ea530eb9823eb28d8573ba06` (2026-01-30), fetched 2026-09-25. Not
transcribed or reconstructed from memory — PC-27's own acceptance criterion is that
`ExportCALM`'s output validates against the **official** schema, so the actual document
had to be pulled and checked, the same "don't guess the schema" discipline
`exporters/fis.go`'s own doc comment already established for AWS FIS.

Vendored (embedded via `go:embed`) rather than fetched at validation time so that
`TestExportCALM_ValidatesAgainstOfficialSchema` (and CI) never depends on network
access or on FINOS's own schema URLs staying live and unchanged underneath this
project.

`core.json` is CALM's actual **document** schema (`nodes`/`relationships`/`metadata`/
`controls`/`flows`/`adrs`) — the one CALM tooling validates real architecture documents
against. `calm.json` (the sibling meta-schema wrapping `core.json` for validating OTHER
schema documents, not architecture instances) is deliberately NOT vendored here — it is
not what `ExportCALM`'s output needs to validate against.

Only `nodes`/`relationships`/`metadata` are populated by `ExportCALM` (`controls`/
`flows`/`adrs` are optional and Preflight has no data to honestly fill them with yet),
so `control.json`/`flow.json` are vendored only because `core.json`'s own schema
structurally `$ref`s them (any JSON Schema compiler resolving `core.json` needs them
present to compile at all, whether or not a given instance document populates those
fields).

To refresh: re-fetch the four files above from the same repo path at a newer release
tag, diff before committing, and update this README's commit SHA/date and any release
version mentioned in `exporters/calm.go`.
