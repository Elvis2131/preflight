# CLAUDE.md — Preflight

## 1. Project identity

**Project name:** Preflight
**Working category:** Architecture assurance control loop for AI-authored infrastructure
**Purpose:** A server engineers — or their AI agents — call during architecture authoring. Each call takes the current IaC state plus a declared workload (NFRs) and returns a versioned assessment: compliance with evidence, a rendered architecture graph, and failure-mode analysis with simulation.

This is a **portfolio/credibility artifact for [[reppl-sh]]**, not a commercial product. Production-grade engineering judgment is the deliverable, not revenue.

**Thesis, one line:** don't just generate infrastructure — prove what happens when it fails.

The core loop is: **author → evaluate → modify → re-evaluate → approve.** Compliance, visualisation, failure analysis, and simulation are capabilities serving that loop, not separate products bolted together.

Source documents (read these before touching code, in this order):
1. PRD (`architecture-assurance-prd.md`) — product thesis, functional scope, golden reference architecture, success criteria
2. Technical Design (`preflight-technical-design.md`) — invariants, process topology, NFRs, technology derivation
3. Architecture Decision Records — ADR-001 (Python, superseded), ADR-002 (SQLite not Neo4j), ADR-003 (three processes not microservices), ADR-004 (Go supersedes ADR-001)

------------------------------------------------------------------------

## 2. Current technology stack

Per ADR-004. Every choice here is derived from a stated NFR in the Technical Design doc §4 — if you're tempted to add a dependency, check whether an NFR actually requires it first.

- Go 1.26+
- `hashicorp/hcl` — HCL parsing (the reference Terraform parser, not a third-party one)
- `gonum/graph` — graph traversal/connectivity (min-cut is hand-implemented; no library gives this)
- `go-playground/validator` + `invopop/jsonschema` (or equivalent) — struct-tag validation and schema generation from the same structs
- `net/http` + `chi` — API layer, SSE streaming
- `github.com/modelcontextprotocol/go-sdk` — official MCP SDK (Google-maintained)
- **No LLM SDK** — `reason/` calls NVIDIA's hosted API directly over `net/http` against the
  OpenAI-compatible chat-completions shape (ADR-005, supersedes the earlier `anthropic-sdk-go`
  assumption). No Anthropic SDK, no OpenAI-compatible client library.
- SQLite (WAL mode) — session/version/finding/ADR/waiver storage
- Graphviz → SVG — deterministic graph rendering
- `golangci-lint`, Go's `internal/` package convention — I1 enforcement

Runtime architecture: **three processes**, not a monolith, not microservices (ADR-003) — see §7.

```bash
go build ./...
go test ./...
go test -cover ./...
golangci-lint run
docker compose up --build   # homelab dev environment
```

------------------------------------------------------------------------

## 3. Product philosophy

### What this project IS

An engine that:
1. Ingests IaC (Terraform/OpenTofu, HCL subset) into a two-level, provenance-tagged intermediate representation.
2. Evaluates that IR deterministically for compliance, structural failure modes, and SPOFs.
3. Simulates faults (node/zone/region/dependency loss) against the IR and reports what survives.
4. Adds LLM narrative *annotation* on top of the deterministic result — never in place of it.
5. Versions every assessment and computes an Assurance Delta between iterations, so an agent can self-correct.
6. Validates its own predictions empirically, at the cheapest rung that answers the question (§13).

### What this project IS NOT

Do not turn this into:
- an IaC generation platform ("AI writes your Terraform") — crowded, commoditised, and off-thesis. Export only happens *after* assurance, never instead of it.
- a drawing/diagramming tool — the architecture graph is **derived from the IR, never hand-edited**. If a canvas is ever built (a deferred idea, see §21), it is an IR *producer* feeding the same unchanged pipeline, not a bypass of it.
- a live cloud account scanner — P0 is static analysis only, no credentials, by design (NFR-15).
- a general compliance-as-code platform — the compliance engine covers what the golden reference architecture needs, not an arbitrary control catalog.
- a chaos-orchestration SaaS — Preflight *predicts* structurally and *exports* runnable experiments; it does not run chaos infrastructure itself beyond the validation ladder's own Rung 3.

The value of this product is **provable, evidenced assurance**, not visual polish and not breadth of cloud-service coverage.

------------------------------------------------------------------------

## 4. Core engineering principle: the five invariants

These are not guidelines. If a future feature requires violating one, the feature does not ship — full stop.

| # | Invariant | Enforcement |
|---|---|---|
| I1 | `core` is pure: no I/O, no network, no LLM, no provider names, no clock, no unseeded randomness | Go's `internal/` package placement (compiler-enforced, unconditional) + `golangci-lint` |
| I2 | Every assertion carries provenance | Provenance is a required field on the base value type; untagged values are unconstructable |
| I3 | `reason` (LLM) annotates, never mutates | Receives a frozen, already-returned assessment; merge is additive, tagged `llm_reasoned` |
| I4 | Incompleteness surfaces as `not_assessable`, never pass or fail | Tri-state resolution on every node/edge; `Assessment[T]` is a sum type, not a nullable bool |
| I5 | Evidence tier cannot be laundered | Emulated (Rung 2) results are typed `functional`, schema-rejected from performance/resilience fields |

------------------------------------------------------------------------

## 5. Cloud behaviour: primary documentation is the source of truth

For AWS/Azure-specific behaviour (failover mechanics, health-check semantics, CIDR/subnet rules, Security Group vs NACL evaluation order):

**Official cloud provider documentation is authoritative. An LLM's training memory is not.**

Priority order:
1. Official AWS/Azure service documentation
2. Official API/SDK reference documentation
3. IAM/RBAC and network policy documentation
4. Terraform provider documentation for the resource in question

Do not invent cloud semantics. If a behaviour is uncertain, the correct answer in Preflight is not a guess — it is `not_assessable` with a stated reason (I4). This is not a fallback; it is the product's core integrity claim.

**On the reference implementation:** an MIT-licensed, browser-based AWS network-physics simulator (CIDR allocation, subnet/VPC containment, Security Group/NACL evaluation order, health-check failover) exists and its logic is being **ported into Go, not vendored** (ADR-004). It is a *design aid*, not ground truth:
- It has had real, dated correctness bugs — a "Multi-AZ implies recovery without explicit surviving capacity" issue was found and fixed there in the same week this decision was made. That is the exact mistake Preflight's capacity-declaration rule (§9) exists to prevent.
- Every rule ported from it must be independently verified against primary AWS/Azure documentation before it lands in `providers/`. Do not trust it, cite it, or treat a passing test in that repo as proof of correctness here.
- Never run it, call it, or depend on it as a runtime component. It is TypeScript, browser-only; Preflight is Go, server-side. A subprocess kept "to avoid rewriting CIDR math" repeats the mistake ADR-002 and ADR-003 already rejected — architecture shaped by wanting to keep code, not by an observed failure domain.

------------------------------------------------------------------------

## 6. Target project structure

Nothing below exists yet (§20). This is the structure to build toward, not a description of a current codebase.

```text
preflight/
├── core/           # Zero I/O, zero LLM, zero provider names. Pure functions.
│   ├── ir/         #   two-level graph schema, resolution states, versioning, diffing
│   ├── analyse/    #   SPOF/min-cut, reachability, capacity, compliance rule engine
│   └── simulate/   #   Layer 1 fault injection (P0) · Layers 2-3 scenario/DES (P2)
├── providers/      # data, not code: aws/ azure/ mappings + capability model
├── ingest/         # HCL parser (defined subset, partial-tolerant) → core.ir
├── reason/         # LLM layer: narratives, contextual controls (annotates only)
├── server/         # net/http + MCP adapter; session store
├── exporters/      # FIS / Chaos Studio / Litmus manifest generators; CALM exporter
├── validate/       # validation-ladder harness: Rung 1 (Toxiproxy/kind), Rung 2
│                   #   (LocalStack), Rung 3 (ephemeral apply/experiment/destroy)
└── ui/             # graph, diff, timeline. Reads server API only. Built last.

contracts/          # six frozen JSON Schemas — see §8
golden/             # the golden reference architecture (Terraform) + fixtures
```

Do not add a top-level package that isn't one of the three processes' concerns (§7). If something doesn't obviously belong in this tree, that's a signal to re-read the PRD before inventing a new one.

------------------------------------------------------------------------

## 7. Process topology — bulkheads by failure domain

Three processes (ADR-003), not a monolith and not microservices:

```text
P1 · Assessment engine   (sync, pure, fast)      — ingest → core → API/MCP.
                                                    No credentials. No external calls.
P2 · Reason worker       (async, external, opt.) — NVIDIA API via net/http (ADR-005),
                                                    annotation only.
                                                    Can be down; product degrades, never fails.
P3 · Experiment runner   (long-lived, credentialed) — validate/ Rung 1+3. THE ONLY
                                                        PROCESS THAT EVER HOLDS CREDENTIALS.
```

Do not add a fourth process without a recorded failure domain that justifies it (ADR-003's revisit trigger: "an observed need, never anticipated"). Do not put credentials anywhere outside P3.

------------------------------------------------------------------------

## 8. The Architecture Model (two-level IR)

Everything hangs off a cloud-agnostic IR, frozen as a contract in week 1:

- **Canonical semantic model** — node types (`compute`, `managed_database`, `cache`, `load_balancer`, `queue`, `object_store`, `dns`, `network_boundary`, `identity`, `external_dependency`), edges (`depends_on`, `routes_to`, `reads/writes`, `authenticates_via`, `replicates_to`, `contained_in`). Cloud-agnostic; what `analyse`/`simulate` reason over.
- **Provider capability model** — typed, per-node capabilities that failure/compliance semantics actually depend on (replication mode, failover mechanism, encryption, multi-AZ implementation). This is where AWS-vs-Azure differences live, not in the canonical layer.
- **Resolution state** on every node/edge: `known` | `inferred` | `unresolved`. Partial input is the *normal* case — agents call mid-authoring. Unresolved never gets silently dropped; it propagates to `not_assessable` (I4).
- **Governing rule: scenario-driven minimalism.** Nothing enters the IR unless one of the six golden failure scenarios (§14) needs it. This is the test for every new field: which scenario needs this? If none, it stays out.

Six frozen contracts in `contracts/`: `ir.schema.json`, `provenance.schema.json`, `workload.schema.json`, `finding.schema.json`, `adr.schema.json`, `canvas.schema.json` (PC-86 — the canvas UI's second IR producer, same freezing discipline as the original five, not an implicit TS→Go shape). Breaking changes require a version bump and migration note (`contracts/CHANGELOG.md`). Everything else in the codebase may churn freely.

------------------------------------------------------------------------

## 9. Provenance and capacity semantics

**Provenance (the epistemic spine).** Every field in every response carries a source tag:

```text
stated        ← from the workload declaration or user input
derived       ← deterministic computation over the IR
assumed       ← a declared default the user can override
llm_reasoned  ← LLM contextual judgment, always citing IR evidence
observed      ← measured in a real experiment (validation ladder Rung 1/3)
```

Untagged assertions must be structurally unrepresentable — not merely discouraged by convention.

**Capacity ≠ count.** Instance *count* is a redundancy fact, derivable from the graph (how many failure domains exist). Node *capacity* is a load fact, only ever from the declaration (can survivors carry peak). Never treat "3 instances" as "3× capacity." Absent declared capacity → the tier is `capacity_unknown` → any capacity finding is `not_assessable`, never assumed, never zero.

**Requirement priority.** `hard` requirements are constraints (violation = failing finding). `preference` requirements are ranked goals (violation = a stated trade-off, not a failure).

------------------------------------------------------------------------

## 10. Failure-mode model

```text
FM-xxx
  trigger, affected_components, blast_radius
  detection            (modeled | declared | unknown | observed)
  existing_mitigation, gap
  impact               (derived: blast radius × workload criticality)
  likelihood           (assumed | unknown — never fabricated; usually unknown)
  detectability        (from the detection field)
  recoverability       (does a failover path exist; is RTO/RPO feasible)
  recommendation
```

**Severity is never a single number.** Impact, likelihood, detectability, recoverability are reported separately. A large-blast-radius/rare-trigger failure is not comparable to a small-blast-radius/common one — collapsing them loses exactly the information a reviewer needs. The same rule applies to the per-version scorecard: **per-dimension, never a composite "architecture score."** False precision invites gaming and hides which dimension actually matters.

`not_assessable` applies to resilience findings, not only compliance ones (e.g., "RTO feasibility: not_assessable — failover duration is provider-managed and no assumption was declared").

------------------------------------------------------------------------

## 11. Simulation layers

**Layer 1 — Structural simulation (P0, the technical centrepiece).** Deterministic fault injection: kill a node/edge/zone/region/dependency. Recomputes reachability, surviving capacity vs `peak_rps` (declared-only), SPOF via min-cut, RTO/RPO feasibility, cascading dependency loss. Handles correlated failure natively via containment closure (an AZ loss takes its contained nodes together — not modelled as independent events).

**Layer 2 — Scenario-based reliability analysis (P2, deliberately demoted).** *Not* Monte Carlo over provider SLAs — that was the original plan, rejected mid-design because published SLAs are contractual credit thresholds, not failure/recovery rates, and infrastructure failures are correlated, not independent. If built, it uses user-declared failure distributions, explicitly tagged `assumed`, and is framed as "under these declared distributions," never as a bare prediction.

**Layer 3 — Discrete-event degraded-state simulation (P2).** SimPy-equivalent request-flow modelling, explicitly conditional on declared assumptions.

Never implement Layer 2 as an SLA-derived Monte Carlo model. That specific approach was tried, reasoned through, and rejected — see the Technical Design doc §4.1's history if you're tempted to revisit it.

------------------------------------------------------------------------

## 12. Compliance engine

Deterministic rules first (Go functions registered by control ID, not an LLM decision) — the LLM is invoked only for controls needing genuine contextual judgment, and every LLM-derived finding must cite the IR evidence it reasoned from.

Output is never a bare pass/fail: `applicable | satisfied | partial | unsatisfied | not-assessable`, each with evidence (resource address, attribute, rationale).

Not OPA/Rego for v1 — the evidence/provenance requirement is the hard part here, not policy expression, and Rego doesn't model `not_assessable` naturally. Revisit only if the control catalog exceeds ~100 controls.

------------------------------------------------------------------------

## 13. Empirical validation ladder

Simulation predicts; experiments verify — at the cheapest rung that answers the question:

| Rung | Environment | Validates | Cannot validate |
|---|---|---|---|
| 1 | Local replica (kind/docker-compose + Toxiproxy) | App behaviour under dependency failure | Cloud-native semantics, real capacity |
| 2 | Emulated control plane (LocalStack) | The tool itself — parser/IR/wiring | Anything performance- or resilience-shaped |
| 3 | Ephemeral real cloud (apply → experiment → capture → destroy) | Cloud-native failure semantics | Capacity claims at declared peak (scaled-down ≠ full load) |

**Hard rule, schema-enforced (I5): an emulated (Rung 2) run must never populate a performance or resilience field.** Emulators are control-plane fakes, not data-plane replicas — LocalStack's "RDS" is a local Postgres with no real failover machinery. Any latency figure from an emulated run is a host-hardware artifact, not a property of the architecture.

*Emulators answer "is it wired right?" Simulation answers "what should happen given assumptions?" Real cloud answers "what does happen?"* No answer crosses those boundaries.

------------------------------------------------------------------------

## 14. Golden reference architecture

One payments workload, modelled exceptionally, rather than broad shallow coverage — this is also the capability vocabulary the product commits to supporting:

```text
Payments API — tier1, PCI, 99.95%, RTO 60s, RPO 0

AWS                          Azure
├── Route53        (dns)     ├── Azure DNS
├── WAF                      ├── Front Door
├── ALB      (load_balancer) ├── Application Gateway
├── EKS  (container_workload)├── AKS
├── RDS   (managed_database) ├── Azure SQL
├── ElastiCache    (cache)   ├── Azure Cache for Redis
├── SQS            (queue)   ├── Service Bus
└── IAM         (identity)   └── Entra / RBAC
```

Six failure scenarios made exceptional: **AZ loss, database failure, queue failure, identity/credential failure, region loss, external-dependency (payment rail) outage.** Coverage is capability-based, not resource-count-based — the product does not claim to support arbitrary Terraform, and a resource outside this vocabulary surfaces as `not_assessable`, never silently ignored.

------------------------------------------------------------------------

## 15. HCL ingestion scope

Defined subset, stated honestly rather than implied broadly:

- **Parsed:** resource/data blocks, module calls, variables/locals, static references.
- **Out of scope for v1 (surfaced as `unresolved`, never a parse error):** `count`/`for_each` expansion, `dynamic` blocks, computed values, complex interpolation.
- **Upgrade path (P1):** `terraform plan` JSON arrives pre-expanded and pre-resolved, lifting these limits without changing the IR.

**Minimum Viable Graph check:** fewer than 1 entry point OR fewer than 1 stateful node → a structured `{status: "insufficient_model", missing: [...], guidance: [...]}` response, never a wall of `not_assessable` findings on a fragment too small to reason about.

------------------------------------------------------------------------

## 16. Architecture Decision Records

Source of truth: the ADR log, not this file — check it before assuming a decision, and never silently reopen one without recording a trigger.

- **ADR-001** (superseded) — Python 3.12. Kept for history: it records real, still-true reasoning about why Rust and Go originally lost.
- **ADR-002** — SQLite (WAL), not Neo4j. The provenance model (`{value, provenance, evidence[]}`) is incompatible with flat property-graph properties; versioning and determinism both favour immutable documents.
- **ADR-003** — three processes split by failure domain, not microservices. A bulkhead around something that cannot partially fail (pure in-memory graph analysis) is cost without benefit.
- **ADR-004** — Go supersedes ADR-001. Triggered by a candidate reference implementation surfacing mid-design (§5); decided on Go's own merits (official SDKs verified for both Anthropic and MCP, `internal/` giving I1 a compiler-enforced guarantee) once the language question was reopened, not because it lets the reference project's code run as-is.

If you find yourself arguing for Neo4j, microservices, Python, or an SLA-derived Monte Carlo model: that argument has already been made and rejected with reasoning on record. Read the ADR before repeating it.

------------------------------------------------------------------------

## 17. Testing philosophy

Four tiers, each protecting something specific:

1. **Unit (`core`, ≥90% line, 100% on simulation/resolution-state paths).**
2. **Golden-file snapshots** — reference Terraform → expected IR → expected findings/simulations, byte-compared. The regression backbone.
3. **Property/mutation tests** — delete-a-resource sweep asserting `not_assessable` propagation (never a false pass/fail); untagged-value construction attempts that must fail; adversarial prompt-injection fixtures asserting verdicts are unchanged.
4. **Acceptance — the agent-iteration test (the primary success criterion, not a smoke test).** The golden architecture ships deliberately broken. A scripted agent, consuming only the Assurance Delta, must reach the target state within a bounded iteration count. **If the agent can't self-correct from the output, the output isn't good enough — fix the output shape, not the agent.**

Determinism (NFR-1) is disciplinary, not automatic: Go maps have deliberately randomized iteration order. Sort before any traversal that feeds output ordering. A CI hash-comparison gate exists specifically to catch regressions here.

------------------------------------------------------------------------

## 18. Anti-patterns

### Provider conditionals in code
```go
if provider == "aws" { ... } else if provider == "azure" { ... }
```
Provider mappings are **data** under `providers/<cloud>/`, not code branches. Adding a cloud means adding a folder, never touching `core`.

### LLM-owned verdicts
`reason/` must never decide pass/fail, SPOF status, or a compliance result. It receives an already-serialised, already-returned assessment and adds narrative only (I3). If a change makes the LLM's opinion capable of altering a finding, revert it.

### Inferred capacity
Never derive "survivors can handle peak load" from instance count alone. No declared capacity → `capacity_unknown` → `not_assessable`. This is the single most tempting shortcut to take by accident; it is also the exact bug the reference implementation shipped and had to fix (§5).

### Composite scoring
No `architecture_score: 87`. No combined severity number. Every scorecard and every failure-mode entry stays per-dimension.

### Vendoring the reference implementation
Never add the TypeScript reference project as a runtime dependency, subprocess, or sidecar. Port verified logic into Go. See §5 and ADR-004.

### Treating incompleteness as failure
A missing attribute, an unresolved reference, or a sub-MVG fragment is `not_assessable`, never a passing or failing verdict.

### Emulator result laundering
A LocalStack round-trip result must never populate a field also used for real-cloud-observed or simulated data. Keep the types separate at the schema level, not by convention.

------------------------------------------------------------------------

## 19. Development workflow for AI agents

1. Read the relevant frozen contract in `contracts/` before writing code that touches it.
2. Check whether the behaviour you're implementing is already decided in an ADR (§16) — don't re-litigate.
3. For any cloud-specific behavioural rule, verify against primary AWS/Azure documentation (§5), not memory, and not the reference project's code unverified.
4. State the intended rule in plain language before implementing it.
5. Add or update a golden fixture or property test for the behaviour.
6. Implement the smallest change that satisfies it.
7. Run the full test suite (`go test ./...`), not just the new test.
8. Confirm determinism: run the affected test twice, hashes must match.
9. Confirm provenance: every new field you introduce has an explicit tag in the schema.
10. If the change touches a frozen contract, bump its version and write the migration note.
11. If the change reopens a settled decision (§16), that's an ADR, not a code comment — write it.

------------------------------------------------------------------------

## 20. Current project status

**Nothing in §6 exists yet.** Unlike a codebase being incrementally upgraded, this file describes a system to build from the frozen contracts outward, not an existing product to preserve.

What is settled and should not be re-derived from scratch by an agent:
- The PRD, Technical Design doc, and four ADRs (§16) — product thesis, NFRs, and language/storage/topology decisions are made.
- The Jira backlog (project key `PC`): five epics — Foundations & Governance, Phase 1 (The Spine), Phase 2 (Assurance), Phase 3 (The Proof), Acceptance & Demo — with stories carrying Card/Conversation/Confirmation and explicit dependency links (PC-15, the golden fixture, blocks the largest number of downstream stories — treat it as the highest-leverage early task, not a late one).

What is not yet decided and should surface as a question, not a silent assumption:
- Whether "environment" (dev/staging/prod) becomes a first-class dimension of `workload.yaml` (this gates any future Terragrunt-style export work — do not add it without raising the question explicitly).
- The exact CIDR/subnet floor for the Minimum Viable Graph check (§15) — needs tuning against real fragments once the parser exists.
- Cross-cloud journey modelling (e.g., DNS failover from AWS to Azure) — explicitly deferred, not silently in scope.

------------------------------------------------------------------------

## 21. Deferred: visual canvas (not in scope for v1)

A drag-and-drop canvas for authoring architectures (rather than writing Terraform) has been discussed and deliberately deferred, sequenced *after* the agent-iteration acceptance test (§17.4) proves the core loop works headlessly. If picked up later:
- It is a new IR **producer** (`POST /sessions/{id}/canvas`), symmetric to `ingest/`, feeding the *same unchanged* `core` pipeline — never a second source of truth, never a way to hand-edit an assessment (§3).
- The palette is restricted to the golden vocabulary's node types (§14) — not "every AWS/Azure service," which would recreate the exact crowded diagramming-tool category this project deliberately isn't.
- A candidate reference for the canvas UI layer (React/`@xyflow`) and its AWS network-physics logic exists — see §5's caveats before using any of it.

Do not start this before Phase 1-3 (§6, §20) are complete. A working canvas on top of a broken or absent assurance engine is a worse artifact than a headless engine that actually proves its claim.

------------------------------------------------------------------------

## 22. Definition of success

1. **The agent-iteration acceptance test passes** — an AI agent, given only the Assurance Delta from each call, drives the deliberately-broken golden architecture to a target state (zero tier-1 SPOFs, all hard requirements satisfied) within a bounded number of iterations.
2. **A recorded end-to-end demo** across both clouds, including a simulated region loss subsequently verified by an exported, runnable chaos experiment.
3. **A predicted-vs-observed writeup** for at least 3 of the six golden scenarios, including **at least one prediction the simulation got wrong and the correction** — this is the single strongest credibility signal in the whole project. A tool that only reports successes is less trustworthy than one that shows its own error-correction process.
4. **The IR design note stands alone** as a technical artifact, readable without the PRD or Design doc alongside it.
5. **A senior SRE reading the repo concludes the author understands failure, evidence, and the limits of LLM judgment.** Every other criterion exists in service of this one.

------------------------------------------------------------------------

## 23. Final instruction to AI agents

> This is a from-scratch build against a deliberately narrow, frozen specification — not a broad platform to extrapolate toward.

The specification is narrow on purpose. Every time this project's history considered broadening scope (multi-cloud beyond two, arbitrary Terraform support, a composite score, an SLA-derived reliability model, microservices, a graph database, vendoring a reference implementation), the broader option was examined and explicitly rejected in favour of the narrower one. Treat a request to relax one of these constraints as a request to reopen an ADR, not a routine implementation choice.

Prefer:
```text
Read the frozen contract
→ Verify cloud behaviour against primary docs
→ Write the test first
→ Implement the smallest change
→ Confirm determinism and provenance
→ Update the contract version if needed
```
over:
```text
Extrapolate from the pattern
→ Hope it matches the spec
```

When a cloud behaviour is uncertain: `not_assessable`, not a guess. When a decision has already been made: cite the ADR, don't re-derive it. When something is only partially implemented: say so in the code and the schema — never imply broader coverage than exists.

The most important quality attributes, in order: **provenance integrity, determinism, evidenced correctness, testability, and honesty about the limits of what the LLM layer can be trusted to decide.**