# ADR-003: Deployment topology — three processes

- **Status:** Accepted
- **Date decided:** not recorded in the available source
- **Ratified into this log:** 2026-09-20 (PC-10)
- **Provenance:** reconstituted from the PC-10 issue record and CLAUDE.md §7. No
  pre-existing ADR-003 document was available.

## Context

Preflight has three workloads with genuinely different failure domains and wildly
different trust requirements:

- deterministic assessment, which must be fast and must never touch the network;
- LLM narration, which is slow, external, and allowed to fail;
- chaos experiment execution, which requires real cloud credentials.

Running these in one process puts credentials in the same memory as the pure analysis
engine and couples the product's availability to an optional LLM provider.

## Decision

**Three processes — not a monolith, not microservices.**

```
P1 · Assessment engine    (sync, pure, fast)          ingest -> core -> API/MCP
                                                      No credentials. No external calls.
P2 · Reason worker        (async, external, optional) LLM annotation only.
                                                      Can be down; product degrades, never fails.
P3 · Experiment runner    (long-lived, credentialed)  validate/ Rung 1 + Rung 3.
                                                      THE ONLY PROCESS THAT EVER HOLDS CREDENTIALS.
```

The split is **by failure domain**, which is what distinguishes it from microservices:
three processes exist because there are three distinct ways this system can fail, not
because three is a nice number of services.

## Rationale

- **The credential boundary must be structural, not aspirational.** PC-10's Card states
  the point directly: scaffold the topology from day one "so that the credential
  boundary (P3-only) is structural rather than retrofitted after something's already
  been built wrong." A boundary added after the fact is a boundary that has already
  been crossed.
- **P1 stays pure.** Invariant I1 (CLAUDE.md §4) requires `core` to have no I/O, no
  network, no LLM, no provider names, no clock, no unseeded randomness. A separate
  process makes the strongest version of that guarantee available.
- **P2 failure degrades, never fails.** The deterministic payload returns regardless of
  LLM availability (see PC-21: killing the LLM connection mid-request must still return
  a complete deterministic payload with a degraded flag).

## Rejected alternatives

- **Monolith.** Puts cloud credentials in the same process as the pure analysis engine
  and couples availability to the LLM provider.
- **Microservices.** Rejected as scope inflation: it multiplies operational surface
  without a matching number of observed failure domains.

## Consequences

- Three runnable entry points must exist from the start, even if P2 and P3 are near-empty
  stubs (PC-10 acceptance criteria).
- Credentials are configured for P3 only. No other process reads cloud credentials.

## Enforcement — resolved

PC-10 originally recorded this honestly as an open gap: I1's purity has a compile-time
enforcement path (Go's `internal/` package placement, per ADR-004), but nothing
enforced P3 being the only credential holder. PC-10 stated this "needs a check, not
just discipline."

That check now exists: `cmd/runnerd/internal/creds` — the same `internal/` mechanism
this ADR already uses for I1, applied to the P1/P2/P3 boundary itself. A package
outside `cmd/runnerd/` cannot import it (compiler-enforced, proven by
`cmd/runnerd/internal/creds/boundary_test.go`'s own synthesize-and-build test, the same
technique `core/boundary_test.go` already established for I1). Nothing lives in
`creds` yet — no real cloud-credential-loading code has been written (Rung 2/3 of
`validate/`, PC-25, aren't built) — the point, per `cmd/runnerd/main.go`'s own comment,
is standing the boundary up structurally before there is content inside it worth
protecting, exactly as this ADR's own topology was stood up before P1/P2's real logic
existed.

A second, live check (`TestP1AndP2HaveZeroCloudSDKOrValidateImports`, same file) proves
today's actual dependency graph: `go list -deps` on `cmd/assessd` and `cmd/reasond`
contains no real cloud SDK module (AWS/Azure/GCP) and no dependency on `validate/`
itself (P3's own designated home) — an active, CI-enforced guard against the moment
either boundary is first crossed, not merely "not violated because not built."

Note also that PC-10's acceptance criteria say P1 purity is "verified by import-linter".
`import-linter` is a Python tool and predates ADR-004; the Go mechanism is `internal/`
placement plus `golangci-lint` (CLAUDE.md §4). See ADR-004's consequences.

## Revisit trigger

Do not add a fourth process without a **recorded, observed** failure domain that
justifies it. ADR-003's own trigger, per CLAUDE.md §7: "an observed need, never
anticipated."
