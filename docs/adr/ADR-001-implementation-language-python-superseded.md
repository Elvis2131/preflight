# ADR-001: Implementation language — Python

- **Status:** **Superseded** by [ADR-004](ADR-004-implementation-language-go-supersedes-adr-001.md)
- **Date decided:** not recorded in the available source
- **Ratified into this log:** 2026-09-20 (PC-8)
- **Provenance:** reconstituted from the PC-8 issue record. The original ADR-001 document
  was not available when this log was created; see "Recovery gap" below.

## Context

Preflight needed an implementation language for a system that is majority pure graph
analysis (`core/`), with an HCL ingestion layer, an HTTP/MCP surface, and an
experiment runner that holds cloud credentials.

Python was chosen. Rust and Go were both evaluated and rejected at the time.

## Decision

Implement Preflight in Python.

## Why this is kept

PC-8 is explicit: ADR-001 is **superseded, not deleted**. It stays in the log marked
Superseded because it records real reasoning — why Rust and Go genuinely lost on the
evidence available then — and that reasoning remains true even though the decision
changed. Deleting a superseded ADR destroys the record of *what was known when*, which
is the only thing that makes a later reversal auditable rather than arbitrary.

## Recovery gap

**The original comparative rationale is not recovered.** The PC-8 record states that
Rust and Go were evaluated and "genuinely lost", but does not state on what grounds.
Those grounds are not reproduced here, because inventing them would fabricate project
history — precisely the failure mode CLAUDE.md §5 and §23 exist to prevent.

If the original ADR-001 document is located, replace this file with it and keep the
Status line pointing at ADR-004. Until then, this file records the decision and its
supersession, and is explicit that the reasoning behind it is missing.

## Consequences

- Superseded. See ADR-004 for the current language decision and the trigger that
  reopened it.
- No Python code is expected to exist in the repository.
