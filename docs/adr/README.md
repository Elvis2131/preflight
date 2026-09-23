# Decision log

Architecture Decision Records for Preflight. An ADR here is **the** record of a decision:
per CLAUDE.md §23, when a decision has already been made, cite the ADR rather than
re-deriving it, and treat a request to relax one as a request to reopen the ADR rather
than as a routine implementation choice.

| ADR | Decision | Status |
|---|---|---|
| [001](ADR-001-implementation-language-python-superseded.md) | Implementation language: Python | **Superseded by 004** |
| [002](ADR-002-storage-sqlite-not-neo4j.md) | Storage: SQLite (WAL), not a graph database | Accepted |
| [003](ADR-003-deployment-three-processes.md) | Deployment: three processes, split by failure domain | Accepted |
| [004](ADR-004-implementation-language-go-supersedes-adr-001.md) | Implementation language: Go 1.26+ | Accepted |
| [005](ADR-005-llm-provider-nvidia-direct-http-not-sdk.md) | LLM provider: NVIDIA API, direct `net/http`, no SDK | Accepted |

Superseded ADRs stay in the log. Deleting one destroys the record of what was known when,
which is the only thing that makes a later reversal auditable rather than arbitrary.

## Revisit triggers, collected

Surfaced here as well as in each ADR, because a trigger nobody can find is not a trigger.

- **ADR-002 (SQLite):** live-account discovery moving from P2 to committed scope, **or**
  estates exceeding a comfortable in-memory graph size.
- **ADR-003 (three processes):** a fourth process requires a *recorded, observed* failure
  domain — "an observed need, never anticipated."
- **ADR-005 (NVIDIA, no SDK):** rate limiting becoming a product complaint; `reason/`
  being proposed to do anything beyond annotation (would violate I3); or the
  OpenAI-compatible shape ceasing to be a reliable common denominator.

## Provenance of this log

**These five files were reconstituted on 2026-09-20 from the Jira issue record** (PC-8,
PC-9, PC-10, PC-77), not from pre-existing ADR documents. The documents those tickets
cite under `docs/adr/` did not exist in any working tree at that time.

Each file states this in its own Provenance line, and each is explicit about what its
source ticket did **not** determine. The most significant gap is in ADR-001: the original
comparative reasoning for why Rust and Go lost to Python is not recovered, and has been
left missing rather than invented.

If any original document is located, replace the corresponding file with it.
