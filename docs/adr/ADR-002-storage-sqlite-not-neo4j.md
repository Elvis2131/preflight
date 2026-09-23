# ADR-002: Storage — SQLite, not a graph database

- **Status:** Accepted
- **Date decided:** not recorded in the available source
- **Ratified into this log:** 2026-09-20 (PC-9)
- **Provenance:** reconstituted from the PC-9 issue record (Card, Conversation,
  Confirmation) and CLAUDE.md §2. No pre-existing ADR-002 document was available.

## Context

Preflight stores sessions, IR versions, findings, ADRs and waivers. The domain is a
graph — architecture nodes and edges — which makes a property-graph store (Neo4j) the
obvious-looking choice. It was rejected.

The question recurs often enough mid-build ("why aren't we using a graph database?")
that PC-9 exists specifically to stop it being re-asked.

## Decision

**SQLite in WAL mode** is the storage backend, holding IR versions as immutable
documents. No graph database.

Graph traversal happens in memory in `core/` (`gonum/graph`), not in the store.

## Rationale

1. **Provenance is incompatible with a property graph.** Every assertion in Preflight
   carries provenance (invariant I2, CLAUDE.md §4) — provenance is a required field on
   the base value type, and untagged values are unconstructable. A property graph wants
   properties on nodes and edges; it does not naturally carry a required, typed
   provenance envelope on every value. This is recorded in PC-9 as the **strongest**
   argument, not merely one of several.
2. **Versioning favours immutable documents.** The product's core loop is
   author → evaluate → modify → re-evaluate, and the Assurance Delta (PC-19) is a diff
   between two IR versions. Diffing two immutable documents is straightforward;
   diffing two states of a mutable graph store is not.
3. **Determinism.** An immutable document read back yields exactly what was written.
   Determinism is a stated product quality (CLAUDE.md §23), and a mutable graph store
   makes it something you have to defend rather than something you get.

## Rejected alternatives

- **Neo4j / property graph store.** Rejected on the provenance-model incompatibility
  above, plus the versioning and determinism arguments. Also adds an operational
  dependency that the three-process topology (ADR-003) does not otherwise need.

## Consequences

- Graph algorithms are implemented in-process against an in-memory graph. The min-cut
  gap this creates is tracked against PC-14 (see ADR-004).
- Estate size is bounded by what fits comfortably in memory.
- Sessions, versions, findings, ADRs and waivers share one embedded store with no
  external service to run.

## Revisit trigger

Reopen this ADR if **either** of the following becomes true:

1. Live-account discovery moves from P2 (speculative) to committed scope.
2. Estates routinely exceed a comfortable in-memory graph size.

Per PC-9's acceptance criteria this trigger must be visible outside this ADR — it is
also recorded in the repository README.

## Open follow-up

PC-9 notes the provenance-incompatibility argument is "worth a sanity check against real
IR fields once they exist" — i.e. after PC-11 designs the two-level IR. That check has
not been done; this ADR is ratified on the structural argument, not on a field-level
audit.
