# ADR-004: Implementation language — Go (supersedes ADR-001)

- **Status:** Accepted. **Supersedes** [ADR-001](ADR-001-implementation-language-python-superseded.md)
- **Date decided:** not recorded in the available source
- **Ratified into this log:** 2026-09-20 (PC-8)
- **Provenance:** reconstituted from the PC-8 issue record and CLAUDE.md §2/§4/§5. No
  pre-existing ADR-004 document was available.

## Context

ADR-001 selected Python. The decision was reopened mid-build by a specific trigger, not
by preference drift.

**The trigger:** an MIT-licensed, browser-based AWS network-physics simulator surfaced —
CIDR allocation, subnet/VPC containment, Security Group vs NACL evaluation order,
health-check failover. It is a credible source of *design* for exactly the rules
`providers/` needs, which forced the question of whether to vendor it or port it, and
that in turn reopened the language question.

## Decision

**Implement Preflight in Go (1.26+).** ADR-001 is superseded.

The reference implementation's logic is **ported into Go, not vendored.**

## Rationale

1. **`internal/` enforces I1 at compile time, unconditionally.** Invariant I1 requires
   `core` to be pure — no I/O, network, LLM, provider names, clock, or unseeded
   randomness. Go's `internal/` package convention makes an illegal import a *compile
   error*, not a lint rule that CI might skip. This replaces the import-linter CI check
   originally planned under ADR-001. An invariant CLAUDE.md §4 describes as "not
   guidelines — if a future feature requires violating one, the feature does not ship"
   deserves compiler enforcement rather than a CI gate.
2. **The MCP layer costs nothing.** Verified, not assumed:
   `github.com/modelcontextprotocol/go-sdk` — the official, Google-maintained SDK — has
   full parity with the MCP story Python would have had.
3. **Port, don't vendor.** The reference implementation is TypeScript and browser-only;
   Preflight is Go and server-side. Keeping it as a subprocess "to avoid rewriting CIDR
   math" would shape the architecture around wanting to keep code rather than around an
   observed failure domain — the same reasoning ADR-002 and ADR-003 already rejected.
   Porting removes the temptation entirely.

## On the reference implementation

It is a **design aid, not ground truth** (CLAUDE.md §5):

- It has had real, dated correctness bugs. A "Multi-AZ implies recovery without explicit
  surviving capacity" defect was found and fixed there in the same week this decision was
  made — the exact mistake Preflight's capacity-declaration rule exists to prevent.
- Every rule ported from it must be **independently verified against primary AWS/Azure
  documentation** before it lands in `providers/`.
- Never run it, call it, or depend on it as a runtime component.

## Rejected alternatives

- **Stay on Python (ADR-001).** Rejected: I1 enforcement stays a CI lint rather than a
  compile-time guarantee, and the vendor-vs-port question resolves less cleanly.
- **Vendor the reference implementation.** Rejected on the architecture-shaped-by-code
  reasoning above. CLAUDE.md §18 lists "vendoring the reference implementation" as a
  named anti-pattern.
- **Rust.** Evaluated under ADR-001 and rejected; the grounds are not recovered (see
  ADR-001's recovery gap).

## Consequences — accepted costs

- **No built-in min-cut.** NetworkX's min-cut has no Go equivalent. **PC-14 implements
  Stoer-Wagner or a max-flow-derived cut by hand.** This is an accepted, named cost and
  is tracked against PC-14 rather than left implicit.
- **Downstream criteria written for Python are stale.** Two known cases, both predating
  this ADR:
  - PC-6 and PC-10 require `import-linter`. Superseded: `internal/` + `golangci-lint`.
  - PC-7 requires schemas "exported from a Pydantic model". Superseded: the Go
    equivalent per CLAUDE.md §2 is `invopop/jsonschema` with `go-playground/validator`,
    generating schemas from the same structs.
- The stack follows from this decision: `hashicorp/hcl`, `gonum/graph`, `net/http` + `chi`,
  SQLite (ADR-002), Graphviz for rendering.

## Note on a superseded example

PC-8's record originally cited `anthropic-sdk-go` alongside the MCP SDK as evidence of Go
ecosystem parity. That specific example is itself now superseded by
[ADR-005](ADR-005-llm-provider-nvidia-direct-http-not-sdk.md): `reason/` uses no SDK at
all. **The underlying point — Go's ecosystem reaches parity for what Preflight needs —
stands; the example does not.**
