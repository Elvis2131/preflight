# Preflight

An architecture assurance control loop for AI-authored infrastructure.

A server that engineers — or their AI agents — call during architecture authoring. Each
call takes the current IaC state plus a declared workload (NFRs) and returns a versioned
assessment: compliance with evidence, a rendered architecture graph, and failure-mode
analysis with simulation.

> **Thesis:** don't just generate infrastructure — prove what happens when it fails.

The core loop is **author → evaluate → modify → re-evaluate → approve.** Compliance,
visualisation, failure analysis and simulation are capabilities serving that loop, not
separate products bolted together.

See [CLAUDE.md](CLAUDE.md) for the full specification. Backlog: Jira project `PC`.

**[Read the IR design note](docs/IR_DESIGN_NOTE.md)** (PC-16) — a standalone writeup
of the four load-bearing design choices behind the IR, readable without this repo's
other docs. Repo-published for now; external publication (Field Notes / blog) is an
open question left to the user, not decided here.

**[Read predicted vs. observed](docs/PREDICTED_VS_OBSERVED.md)** (PC-26) — three
golden failure scenarios, including two real, documented misses (each with an actual
engine fix, not a caveat) and the one pattern connecting both.

## Status

Early. Phase 1 (The Spine) is in progress; most of the tree in CLAUDE.md §6 does not
exist yet.

| Component | State |
|---|---|
| Decision log (`docs/adr/`) | ADR-001…005 ratified |
| Golden reference architecture (`golden/`) | AWS side authored, `terraform validate` passing (PC-15) |
| Go module + 3-process skeleton | scaffolded: `cmd/assessd` (P1), `cmd/reasond` (P2), `cmd/runnerd` (P3) all build and run (PC-10) |
| Invariants I2, I4 | type-level enforcement + tests in `core/` (`Provenance`/`Tagged[T]`, `Assessment[T]`) (PC-6) |
| Invariant I1 (structural half) | `core/internal/{ir,analyse,simulate}` compiler-enforced private; proven by `core/boundary_test.go`, not just asserted |
| IR design (`core/ir.go`, `docs/IR_DESIGN_NOTE.md`) | canonical/capability boundary resolved for all 8 golden node types + WAF's edge case; resolution-state propagation structurally enforced via `core/internal/analyse.AssessNode`/`AssessEdge` (PC-11) |
| Invariant I1 (defense-in-depth half) | `.golangci.yml` (depguard + forbidigo), migrated to the v2 schema and actually run for the first time (PC-6) — running it surfaced and fixed 3 real config bugs (a glob that never matched direct children of `core/`, a misunderstood `pkg` field), each caught by negative control; `.github/workflows/ci.yml` runs it in CI |
| Contract schemas (`contracts/`) | all 6 generated from Go structs + validated against real samples (PC-7 froze the first 5; `canvas.schema.json` 1.0.0 added PC-86, decided explicitly before canvas ingestion was built rather than left implicit); `finding.schema.json` at 1.2.0 (1.1.0 `FailureMode` dimensions, PC-17; 1.2.0 `EvidenceRef.Attribute`, PC-18), others unchanged at 1.0.0 — schemas version independently per Design §5, see `contracts/CHANGELOG.md` |
| `ingest/` HCL parser | real HCL v2 parsing, MVG check, golden/aws round-trips losslessly (64 resources: 11 nodes + 1 mapped edge + 52 out-of-vocabulary) (PC-12) — WAF↔ALB association edge captured via `providers/aws`'s edge-mapping mechanism; clean/broken bundles verified to differ on it |
| Golden fixtures (`golden/fixtures/`) | IR **and** findings fixtures, derived + hand-verified for both bundles (PC-15, complete) — 5 representative findings per bundle from the real PC-14/17/18/28 engines, including a checked not_assessable case |
| Version diffing / Assurance Delta (`core.ComputeDelta`, `core.Scorecard`) | 6-value `DeltaKind` enum (PC-83 added `resolved_risk`, PRD §5.7's own category, distinct from `improvement`: an unknown resolving into known-good vs. a known state getting better), every entry carries `Provenance`, no composite score anywhere (checked via reflection) — all 6 kinds hand-verified synthetically, `improvement`/`resolved_risk` also proven against the real golden bundles (PC-19, PC-83) |
| Layer 1 structural analysis (`core/internal/analyse`) | min-cut, SPOF detection, capacity discipline, zone-loss/containment reachability, RTO/RPO feasibility — all hand-verified in isolation before touching real data (PC-14). 2 real bugs found and fixed this way |
| `providers/aws/` | 8 golden resource types + VPC/subnet/NAT/security-group substrate (PC-78) mapped (YAML data + loader), every field's provenance explicit, 4 assumed defaults doc-verified (PC-13); zone-kill now reports real, hand-verified losses on the golden bundle (PC-78); own `FailoverMapping` data now carries doc-verified replication/failover wording per resource (PC-22 groundwork — relocated out of `core/internal/analyse`, which must stay provider-agnostic, before it could have handed Azure SQL AWS's own wording verbatim) |
| `providers/azure/` + `golden/azure/` + `golden/azure-broken/` | 8 golden resource types mapped and real, `terraform validate`-passing golden bundles built on them (PC-22, PC-29) — Azure DNS, App Gateway + its WAF policy, AKS, Azure SQL, Cache for Redis, Service Bus, RBAC role assignment; Front Door explicitly out of scope; shares `providers/`'s schema with AWS. `core.BuildFindings` and `server.Assess` both used to be hardcoded to AWS (node IDs/attribute keys; the AWS-only provider registry) — **fixed under PC-29**: genericized to run against any `managed_database` node (AWS's own finding IDs regression-tested unchanged) and merged both providers' registries (`providers.Merge`). A real live-agent iteration cycle converges on Azure in 1 round with correct `resolved_risk`/`improvement` classification (`demo/pc29-azure-live-agent-run/`). Azure still has no subnet-level AZ placement to run zone-kill against at all — a real model difference with its own follow-up ticket, not silently produced as working (see `golden/azure/README.md`) |
| `server/` (SQLite store, `/assess`, `/simulate`, `GET /sessions/{id}/versions/{n}`, MCP tools) | real, running server (PC-21): SQLite-backed session/version store (ADR-002, also closes PC-9's remaining criterion), `/assess` measured under 5s, HTTP and MCP proven to call the identical function, `graph` field is real, deterministic server-rendered SVG (PC-81 — `core.RenderDOT` + Graphviz via `render/`, byte-identical output proven across independent runs against the real golden bundle); `/simulate` (PC-82) declares one fault against an already-assessed version, reusing PC-14's fault-injection engines directly — `region_loss` and `node_loss` (PC-88) both hand-verified live against golden/aws. `GET /sessions/{id}/versions/{n}` (PC-94) re-serves a stored assessment with zero re-ingest/re-analyse — the API-side half of canvas's own "no persistence" gap, surfaced by a proactive audit rather than a fourth reactive discovery. Failures are structured now (PC-95): a stable `{error_code, message}` body plus a real status per failure mode (`session_not_found`/`version_not_found` → 404, `invalid_request_body` → 400, `invalid_workload`/`invalid_bundle`/`insufficient_model` → 422) instead of one generic 400 for everything — additive, existing text-matching callers still work unchanged |
| API docs (`/openapi.json`, `/swagger`) | OpenAPI 3.1 spec reflected directly from `AssessRequest`/`AssessResponse` via `invopop/jsonschema` (not hand-written — same discipline as `cmd/gen-contracts`), rendered with Swagger UI 5.29.1 |
| Agent-iteration acceptance test (`server/agent_iteration_acceptance_test.go`) | CI-deterministic: drives a mutable working copy of `golden/aws-broken` through the real author→evaluate→modify→re-evaluate loop via `server.Assess`, applying scripted fixes per unsatisfied finding, and proves convergence in 2 of a written-down 5-iteration bound (PC-28; bound decided up front in `docs/PC-28-ITERATION-BOUND.md`, not discovered empirically). A separate live-agent demo run is PC-29's job, not built here |
| `exporters/` | FIS + Litmus chaos-experiment exporters (PC-23) — real, doc-verified schemas (not guessed), each export carries its own Provenance distinct from the source finding; a region-wide FIS variant (PC-29) verifies `/simulate`'s `region_loss` prediction |
| `canvas/` | Drag-and-drop architecture authoring canvas shell (PC-85, React + `@xyflow/react`) — golden-vocabulary-only palette (11 real `core.NodeType`/6 `core.EdgeType` values, mirrored from `core/ir.go`, not redefined), clean `CanvasDocument` serialization (no UI-only fields — unit-tested against React Flow's own real internal fields), verified live in a real browser (Playwright), not just typechecked. Now wired to the backend (PC-86): `POST /sessions/{id}/canvas` — `ingest.IngestCanvas`, a second IR producer symmetric to HCL ingestion, sharing the identical downstream pipeline (`server.assessFromResult`) so the two paths can't quietly diverge; a dangling edge produces real `not_assessable` findings, never a crash or a guess. `CanvasDocument` is now `canvas.schema.json`, the sixth frozen contract, decided explicitly before this ingestion logic was written — no capability form yet (PC-87) |
| `viewer/` | PRD §6's thin, read-only secondary web UI (PC-90) — reads the API only, authors nothing. PC-92: a per-finding scorecard timeline table (V1..Vn columns), colored by the server's own real `DeltaKind` classification, no composite score/trend line anywhere (reviewed explicitly against CLAUDE.md §10 before writing the one function that builds it). Required a new endpoint, `GET /sessions/{id}/versions` (`server/history.go`) — a real gap found starting this ticket: nothing could answer "how many versions does this session have." Verified live in a real browser against a real 2-version golden broken→clean transition, not just typechecked. PC-91 (graph/diff view) can now build on PC-81's real SVG output — not in scope for PC-92 itself |
| `reason/`, `validate/`, `ui/` | package stubs only (`doc.go`), no logic |

## Layout

```
preflight/
├── CLAUDE.md       full specification
├── docs/adr/       decision log — see below before proposing an architectural change
└── golden/         golden reference architecture (Terraform) + declared workload
    ├── aws/        clean baseline
    └── aws-broken/ deliberately-imperfect variant (8 seeded defects)
```

## Decisions

Architectural decisions live in [`docs/adr/`](docs/adr/). Cite the ADR rather than
re-deriving the decision; treat a request to relax one as a request to reopen it.

| ADR | Decision | Status |
|---|---|---|
| 001 | Implementation language: Python | Superseded by 004 |
| 002 | Storage: SQLite (WAL), not a graph database | Accepted |
| 003 | Deployment: three processes, split by failure domain | Accepted |
| 004 | Implementation language: Go 1.26+ | Accepted |
| 005 | LLM provider: NVIDIA API, direct `net/http`, no SDK | Accepted |

### Revisit triggers

Conditions that reopen a settled decision. Recorded here, outside the decision log, so
they are findable by someone who is not already reading the ADR they belong to.

- **ADR-002 (SQLite, not a graph DB)** — reopen if live-account discovery moves from
  speculative to committed scope, **or** if estates routinely exceed a comfortable
  in-memory graph size.
- **ADR-003 (three processes)** — a fourth process requires a *recorded, observed*
  failure domain. "An observed need, never anticipated."
- **ADR-005 (NVIDIA, no SDK)** — reopen if free-tier rate limiting becomes a product
  complaint rather than an occasional gap; if `reason/` is ever proposed to do more than
  annotate (this would violate invariant I3); or if the OpenAI-compatible request shape
  stops being a reliable common denominator.

## The five invariants

Not guidelines. If a feature requires violating one, the feature does not ship.
See CLAUDE.md §4 for enforcement mechanisms.

| # | Invariant |
|---|---|
| I1 | `core` is pure: no I/O, network, LLM, provider names, clock, or unseeded randomness |
| I2 | Every assertion carries provenance |
| I3 | `reason` (LLM) annotates, never mutates |
| I4 | Incompleteness surfaces as `not_assessable`, never pass or fail |
| I5 | Evidence tier cannot be laundered |

## Working on the Go skeleton

```bash
go build ./...
go test ./...
go vet ./...
```

Three entry points exist and each is independently runnable:

```bash
go run ./cmd/assessd    # P1 — pure assessment engine, HTTP :8080 (PREFLIGHT_ASSESSD_PORT to override)
go run ./cmd/reasond    # P2 — LLM annotation worker stub, no client wired up yet
go run ./cmd/runnerd    # P3 — experiment runner stub, the only process ever permitted credentials
```

`golangci-lint` is not installed in this environment — `.golangci.yml` is drafted but
**has not been run**. Do not treat its presence as evidence I1's defense-in-depth half is
enforced; only `core/boundary_test.go` (the compiler half) has actually been verified.

## Working on the contract schemas

```bash
go run ./cmd/gen-contracts   # regenerates contracts/*.schema.json from core/'s Go structs
go test ./contracts_test/... # validates the generated schemas against contracts/samples/
```

The schemas are generated output, not hand-edited — if a schema looks wrong, fix the Go
struct in `core/` (its tags) and regenerate, never edit the `.schema.json` file directly.
See `contracts/CHANGELOG.md` for the versioning convention. One named gap remains in
the freeze (IR provenance is node-level, not the fully general per-field granularity
PRD §4 describes); `Finding.Dimensions`, the other original gap, was resolved by PC-17
(`finding.schema.json` 1.1.0).

## Working on the parser

```bash
go test ./ingest/...   # includes a real round-trip test against golden/aws
```

`ingest.Ingest(dir, registry, versionNumber)` parses a directory of `.tf` files,
maps resources through `providers/aws`'s registry, runs the Minimum Viable Graph check,
and returns either a real `core.IR` or a structured `InsufficientModel`. A relationship
expressed via a separate "association" resource (e.g. `aws_wafv2_web_acl_association`)
is handled by `providers/aws.EdgeMapping` — declared as data, not a parser special case
— and consumed by `buildMappedEdge` in `ingest/build.go`.

## Working on Layer 1 structural analysis

```bash
go test ./core/internal/analyse/...   # hand-verified, core-independent (no preflight/core import at all)
go test ./core/... -run "SPOF|ZoneKill|RTORPO"   # end-to-end against the real golden bundles
```

`core/internal/analyse` has zero dependency on `preflight/core` by design — every
function takes/returns plain Go types, and `core/assess.go` is the only place results
get wrapped into `core.Assessment[T]`/`core.Provenance`. This is a hard architectural
rule, not a style preference: `core` is the sole public surface onto its own internal
packages (I1), and `core/internal/analyse` importing `core` back would be a real import
cycle — found the hard way while building this.

**PC-78 closed the substrate gap** SPOF/zone-kill were originally blocked on: VPC,
subnet (with real AZ placement), NAT gateway, and security group are now mapped
(`providers/aws/{vpc,subnet,nat_gateway,security_group}.yaml`) with real
`contained_in` edges. `core.ContainmentBlastRadius` (a reverse-reachability query,
distinct from `SimulateLoss` — see its own doc comment for why the two are genuinely
different graph problems) now reports real, hand-verified losses against the golden
bundle — verified as the COMPLETE expected set per AZ (`reflect.DeepEqual`, not just
"contains the node I expected"), across 4 distinct AZ scenarios in both bundles:

- `aws_subnet.public_a` → exactly `[aws_lb.payments, aws_nat_gateway.nat_a]`
- `aws_subnet.private_a` → exactly `[aws_eks_cluster.payments]`
- `aws_subnet.public_b`, clean vs broken → the case that actually shows defect 1
  through zone-kill (broken loses only the ALB placement; nat_b is already gone)
- `aws_subnet.data_a` → empty, and **known to be a real blind spot, not a confirmed
  safe result**: RDS/ElastiCache reach their subnets through `aws_db_subnet_group`/
  `aws_elasticache_subnet_group`, which remain unmapped — see the test's own comment
  in `core/zoneloss_golden_test.go`.

**Remaining, still-honest gap**: entry-to-critical-node SPOF (dns/lb → database/cache/
queue) still correctly reports "no path" — Terraform doesn't encode application-level
connectivity (which security group rules allow) as a resource reference, and SG-to-SG
permission edges are an explicit, recorded scope exclusion of PC-78's own mapping (that
belongs to the Network Engine's SG/NACL evaluation, a separate future engine — see
`providers/aws/security_group.yaml`'s own scope-decision comment). See
`core/spof_golden_test.go`'s doc comment for the full account.

## Working on the four-dimension failure-mode model

```bash
go test ./core/... -run "FailureMode|DetectionState"   # structural + golden-bundle tests
```

`core.FailureMode` (CLAUDE.md §10's FM-xxx record) has four independently
not_assessable-capable dimensions — impact, likelihood, detectability, recoverability —
never combined into one severity number (checked structurally via reflection, not just
by convention). `core.DeriveLikelihood` always returns not_assessable for this analysis
layer — that is the correct, permanent answer per PC-17's own Conversation ("unknown
beats fabricated"), not a gap to fill in later. `Recoverability` reuses PC-14's
`RTOFeasibility`/`RPOFeasibility` directly rather than re-deriving anything.

## Working on the compliance engine

```bash
go test ./core/... -run TestComplianceCheck   # end-to-end against both real golden bundles
```

One control, narrow and correct per this ticket's own recommendation:
`core.RDSStorageEncryptionCheck` (`storage_encrypted`), verified against both golden
bundles (satisfied on `golden/aws`, unsatisfied on `golden/aws-broken`'s defect 2).
Never a bare pass/fail — `core.ComplianceStatus` is a 5-value enum
(`applicable | satisfied | partial | unsatisfied | not_assessable`). Every finding's
evidence carries resource address, attribute, and rationale (`core.EvidenceRef`,
extended in `finding.schema.json` 1.2.0). 100% deterministic — `reason/` has no client
code at all yet, so "the LLM is never touched for deterministic checks" holds
structurally, not by discipline.

**Honestly flagged, not silently decided**: the ticket names "CIS framework," but a
citable CIS AWS Foundations Benchmark control ID for RDS storage encryption was not
found via any accessible source (CIS's own benchmark text is paywalled). Verified
instead via Prowler's public, open-source check metadata, which maps this control to
PCI-DSS/AWS Security Best Practices/NIST/ISO 27001/HIPAA/GDPR — not confirmed CIS. Filed
under the frameworks actually verified rather than an invented CIS control number.

## Working on version diffing and the Assurance Delta

```bash
go test ./core/... -run "ComputeDelta|Scorecard|AssuranceDelta"
```

`core.ComputeDelta` classifies every finding into one of six kinds — never a signed
number or composite score — `improvement | regression | new_risk | unchanged |
not_assessable | resolved_risk` (PC-83 added `resolved_risk`, distinct from
`improvement`: an unknown resolving into known-good, not a known state getting
better), each carrying its own `Provenance`. Scoped deliberately to
compliance-shaped (plain-string) outcomes only: a structural finding's free-form
Outcome (e.g. zone-kill's "2 component(s) affected") has no defined ordering, and
inventing one to force a comparison would be the same fabricated-precision anti-pattern
CLAUDE.md §10 already warns against for severity scoring. All 6 delta kinds are
hand-verified against a small, realistic synthetic scorecard before being run against
the real golden fixtures, which naturally produce only the `improvement` case (both
bundles differ in one consistent direction, not a mixed bag) — `regression`/`new_risk`
are proven correct on the synthetic case, honestly, rather than forced out of two
bundles that don't happen to exercise them simultaneously.

**Open design question, deferred rather than guessed**: this ticket's own Conversation
asks whether `assurance_delta` should be computed lazily on request or stored per
version at write time. Neither a server (PC-21) nor a storage layer (SQLite, ADR-002)
exists yet to answer this with real latency/storage-shape evidence — `ComputeDelta`
itself works identically either way (it's a pure function over two status maps), so the
choice is deferred to whichever ticket actually wires up persistence, not resolved here
by guessing.

## Working on the server

```bash
go test ./server/...
go run ./cmd/assessd   # HTTP :8080 — /healthz, POST /assess, MCP at /mcp (streamable HTTP)
```

`server.Assess` is the ONE implementation both `server.AssessHandler` (HTTP) and the
MCP `assess` tool call — proven by a test that exercises both entry points end to end
against independent stores and compares real output, not by reading the source and
trusting it. SQLite (ADR-002, WAL mode) persists sessions/versions; a session's second
`/assess` call produces Version N+1 with a real `assurance_delta` (PC-19) against N.

**A real bug, caught by actually registering the MCP tool, not by reading the SDK
docs**: `mcp.AddTool`'s generic form reflects the output type's `jsonschema` struct
tags via `google/jsonschema-go`, which parses that tag differently than
`invopop/jsonschema` (used by `cmd/gen-contracts`) does — it panics on the same
`required,minLength=1` syntax invopop treats as entirely valid (confirmed: `contracts/
*.schema.json` correctly has `minLength` applied). Fixed by using `any` for the MCP
tool's output type (per the SDK's own documented behavior) rather than weakening
`core/`'s frozen contract tags to satisfy a second, incompatible schema library.

**Two things named as explicit scope exclusions at the time, not silent gaps**:
- `graph` was always a stub string, never omitted or an error, pending PC-81 — PC-81
  has since landed real, deterministic SVG rendering (see the Status table above).
- `/simulate` (named in this ticket's own title) has no acceptance criteria describing
  its behavior and is not built — inventing one would be scope this ticket doesn't
  actually specify.

## Working on the golden architecture

```bash
cd golden/aws          # or golden/aws-broken
terraform init -backend=false
terraform validate
```

Neither variant has been applied to a real account. See [golden/README.md](golden/README.md)
for the node-type map and the defect inventory.
