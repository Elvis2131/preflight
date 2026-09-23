# Contract changelog

Design §5: "Changes to these require a schema version bump and a migration note.
Everything else in the codebase may churn freely — that asymmetry is the point."

Every entry below states: which schema changed, the version bump, and what a caller
holding a document against the old version needs to do.

Each generated `contracts/*.schema.json` carries its own version in `x-schema-version`,
stamped by `cmd/gen-contracts` — check that field against this file, not the other way
around, since a schema file is regenerated output, not hand-edited.

## canvas.schema.json 1.0.0 — 2026-09-22 (PC-86 groundwork: sixth frozen contract)

**New contract, not a change to the existing five.** `core.CanvasDocument` (nodes +
edges the canvas UI produces, PC-85) is frozen the same way the original five were —
decided explicitly before PC-86's own ingestion logic was built, not left implicit as
"whatever `canvas/src/serialize.ts` currently happens to emit." Without this, a later
change to the canvas (PC-87 adding real capability structure, for instance) could
silently drift from what the Go-side parser expects — the same class of
producer/consumer mismatch already caught and fixed twice elsewhere in this codebase
(the `FailoverMechanismNone` sentinel mismatch, PC-14; the `BuildScorecard`/
`ComputeDelta` `not_assessable` exclusion, PC-83).

`canvas/src/types.ts`'s own `CanvasDocument` is now the mirror of this Go struct — the
same relationship `core/ir.go`'s `NodeType`/`EdgeType` already has with
`canvas/src/goldenVocabulary.ts` (one source of truth, mirrored deliberately, not
independently redefined on the TypeScript side).

## finding.schema.json 1.2.0 — 2026-09-20 (PC-18: EvidenceRef.Attribute added)

**Additive change to `finding.schema.json` only** — the other four schemas are
untouched and remain at their current versions.

`EvidenceRef` gains one new, optional field: `attribute` (the specific field a
compliance check evaluated, e.g. `storage_encrypted`). `node_id` already served as the
"resource address" and `description` already served as the "rationale" — PC-18's own
acceptance criterion ("every finding includes resource address, attribute, and
rationale") needed only this one genuinely new field, not a redesigned evidence shape.

**Migration**: additive and optional (`omitempty`) — every document valid against 1.1.0
remains valid against 1.2.0 unchanged. No migration action required for existing
documents; new compliance findings should populate `attribute` going forward.

## finding.schema.json 1.1.0 — 2026-09-20 (PC-17: Dimensions typed)

**Breaking change to `finding.schema.json` only** — `ir`, `provenance`, `workload`, and
`adr` are untouched and remain at 1.0.0 (per-contract independent versioning; see
`cmd/gen-contracts`, itself corrected in this same change to actually support that
rather than stamping one global version on all five).

`Finding.Dimensions` changes from the 1.0.0 placeholder (`map[string]any`) to
`core.FailureMode` — CLAUDE.md §10's FM-xxx record, real dimensions: `impact`,
`likelihood`, `detectability`, `recoverability`, each independently
not_assessable-capable, plus `trigger`/`affected_components`/`blast_radius`/
`detection`/`existing_mitigation`/`gap`/`recommendation`.

**Migration**: any document holding the old `dimensions: {}` placeholder shape does not
validate against 1.1.0 — there is no backward-compatible reading of an untyped map as
the new typed structure. No documents existed in production against 1.0.0 (this is the
schema's first real use), so no live migration is required; regenerate any document
from source instead of attempting to reshape one that predates this change.

**Also corrected in this pass**: `Finding.Detection` (top-level) is retired. An earlier
draft had a same-named field with an entirely different, invented enum (`structural |
compliance_rule | simulation | live_observation`, guessed before CLAUDE.md §10's real
text was available) answering a different question ("how was this finding computed").
That guess was never released in a schema version anyone depended on (it existed only
inside 1.0.0's development, not a tagged freeze), so this is not tracked as its own
breaking change — but is recorded here for the same reason every other correction in
this log is: so the history of what was guessed versus grounded stays visible.

## 1.0.0 — 2026-09-20 (initial freeze)

All five schemas frozen for the first time: `ir`, `provenance`, `workload`, `finding`,
`adr`. No migration — this is the baseline every future entry diffs against.

Known gaps in this baseline, carried forward rather than silently fixed later without a
record of why they existed:

- **`finding.schema.json`: `Dimensions` is an untyped placeholder.** Design §5 calls for
  a "four-dimension failure model" but no source text available at freeze time named the
  four dimensions. PC-17 owns replacing this field's type — that will be a breaking
  change to `finding.schema.json` (a typed object replacing `map[string]any`), version
  bump required, this file updated then.
- **`ir.schema.json`: `Node.Provenance` is one Provenance per node, not per field.**
  PRD §4 states the general principle as per-field ("every field in every response
  carries a source tag"); this baseline applies it at node granularity as a stated
  simplification. PC-11's full IR design may need per-field provenance, which would be
  a breaking change to `ir.schema.json`'s `Node` shape.
- **`workload.schema.json`: `Requirement.Value` and `Finding.Outcome.Value` are untyped
  (`any`).** PRD §4 does not constrain a requirement's value to one type (a duration, a
  percentage, and a string are all valid requirement values in the examples given), so
  this is a considered choice, not a placeholder — unlike the two gaps above, no future
  PC is expected to "fix" this one away. (`Finding.Dimensions` itself was untyped at
  1.0.0 too, but that one WAS a placeholder — see the 1.1.0 entry above, where PC-17
  replaced it.)
