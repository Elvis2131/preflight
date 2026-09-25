# Preflight PRD

**Source of truth:** Confluence, `Preflight PRD`, page ID `2981890`
(`https://elartey.atlassian.net/wiki/spaces/~5bbb38ce5a05242b81c6e0f0/pages/2981890`).
This file is a synced local copy, created by PC-103 (previously this repo cited
`docs/architecture-assurance-prd.md` from many tickets — PC-4, PC-14, PC-17, PC-20,
PC-24, PC-27, PC-78, PC-79 among them — without the file existing on disk; see those
tickets' own comments and ADR-003's "reconstituted" note on the equivalent Technical
Design gap). If this file and the Confluence page ever disagree, Confluence is
authoritative — update this file to match, don't edit around the discrepancy.

**Working name:** TBD (placeholder: "Preflight")
**Owner:** Elvis / Reppl.sh
**Status:** Draft v6 (PC-103: architect-first pivot — primary user changed from the
platform/SRE engineer with an AI agent to the architect designing on a canvas; the
MCP/agent loop, PC-21/28, stays as a fully supported second interface, not removed or
demoted). Thesis frozen — next work is contract artifacts, not more PRD.
**Headline principle:** every assertion carries provenance — `stated | derived |
assumed | llm_reasoned | observed` — and untagged assertions are unrepresentable. This
is the epistemic spine of the whole system.
**Purpose of build:** Reppl.sh credibility artifact demonstrating reliability
engineering judgment. Commercialisation is not a goal; production-grade thinking is.

---

## 1. Problem statement

AI increases the velocity of infrastructure change faster than architectural assurance
can keep up. Generated IaC is syntactically valid and architecturally unaccountable:
nobody can say, per iteration, what compliance posture it holds, what it looks like as
a system, or how it fails. Existing tooling primarily evaluates finished infrastructure
code (PR bots, upload-and-report tools), deployed environments (AWS Resilience Hub), or
individual policy controls. None makes architecture assurance a first-class, iterative
artifact inside AI-assisted authoring.

**The product:** an architecture assurance control loop. A server engineers (or their
AI agents) call during authoring; each call takes the current IaC state plus a
workload declaration and returns a versioned assessment — compliance with evidence, a
rendered architecture graph, and failure-mode analysis with simulation. The iteration
loop (author → evaluate → modify → re-evaluate → approve) is the product; compliance,
visualisation, failure analysis, and simulation are capabilities serving that loop.

**Thesis, one line:** don't just generate infrastructure — prove what happens when it
fails.

## 2. Cloud-agnostic by principle, multi-cloud as first proof

The product principle is that **assurance must not be coupled to the cloud
provider** — an AWS-only shop has this problem too, and its assurance should survive a
migration. Multi-cloud support is the first proof the abstraction works, and it is
commercially load-bearing for the target segment:

- AWS Resilience Hub (next-gen, GA May 2026) owns AI failure-mode analysis for
  pure-AWS workloads. Competing there is competing with a free hyperscaler feature.
- Azure Well-Architected and GCP Architecture Framework exist but no tool reviews
  against all three under one model.
- Regulated UK/EU financial services — the Reppl.sh audience — are pushed toward
  multi-cloud by FCA operational resilience and DORA concentration-risk expectations. A
  single-cloud tool cannot serve them.
- Terraform providers already give one ingestion path to all three clouds. The hard
  part is the abstraction layer; its long-term value compounds as mappings, controls,
  failure modes, and predicted-vs-observed results accumulate against it.

## 3. Users

**PC-103 (v6): primary user changed.** The architect designing on the canvas (the
Architect Workspace, PC-96/104) is now primary. The platform/SRE engineer authoring
infrastructure with an AI agent — the original v1–v5 primary user — is a fully
supported second interface: the same API, the same MCP/agent loop (PC-21, PC-28),
unchanged in capability. Neither interface is a degraded path to the other; both call
the identical `core` pipeline (§7).

| User | Job to be done | Interface |
|---|---|---|
| Architect designing on the canvas (primary, v6) | "Design here, and tell me what breaks, what fails compliance, what it costs, and how traffic moves — before I ship it" | Architect Workspace: Design / Simulate / Failure Lab / Analyze / Report (PC-96/104) |
| Platform/SRE engineer authoring IaC with an AI agent (primary through v5, still fully supported) | "Tell me what breaks and what fails compliance before I commit" | MCP tool / API called from editor or agent |
| AI coding agent (Claude Code, etc.) | Machine-readable assessment to self-correct against | Same API, structured JSON response |
| Tech lead / architect reviewing | Review iteration N vs N-1; approve with evidence | Analyze mode: graph, diffs, findings, ADRs |
| Compliance/risk stakeholder | Evidence trail mapping controls to resources | Report mode (PC-101) export per version |

## 4. Core object: the Architecture Model

**Governing rule: the IR is minimal and scenario-driven.** Nothing enters the IR unless
one of the six golden failure scenarios (section 10) requires it — or, as of v6, one of
the new epics' own declared inputs requires it (traffic/cost/report add real fields
because PC-98/100/101 name specific new questions the IR must answer, not because they
would be nice to model; the same test applies: which scenario or declared epic need
needs this?). The capability fields below exist because AZ-loss, database-failure, and
region-loss *need* them to be reasoned about. This is the test for every future field:
which scenario needs this? If none, it stays out.

Everything hangs off a cloud-agnostic intermediate representation (IR). Provider
resources normalise to capability-level node types:

```
Node types (examples)
  compute            ← aws_instance | azurerm_linux_virtual_machine | google_compute_instance
  container_workload ← ECS/EKS | AKS | GKE workloads
  managed_database   ← RDS/Aurora | Azure SQL/Flexible Server | Cloud SQL/Spanner
  cache              ← ElastiCache | Azure Cache for Redis | Memorystore
  load_balancer      ← ALB/NLB | Azure LB/App Gateway | GCP LB
  queue/stream       ← SQS/Kinesis | Service Bus/Event Hubs | Pub/Sub
  object_store       ← S3 | Blob Storage | GCS
  dns                ← Route53 | Azure DNS | Cloud DNS
  network_boundary   ← VPC/subnet/SG | VNet/subnet/NSG | VPC/subnet/firewall
  identity           ← IAM | Entra/RBAC | IAM
  external_dependency ← declared third parties (Auth0, Stripe, payment rails)
```

Edges: `depends_on`, `routes_to`, `reads/writes`, `authenticates_via`, `replicates_to`,
`contained_in` (zone/region/network).

**The IR is two-level, not a flat abstraction:**
- **Canonical semantic model** — node types, edges, topology, placement, redundancy.
  Cloud-agnostic; what analysis and simulation reason over.
- **Provider capability model** — typed, per-node capabilities that failure and
  compliance semantics actually depend on: replication mode (sync/async), failover
  mechanism and expected class of failover time, backup semantics, encryption mechanism
  and key ownership, multi-AZ implementation, maintenance behaviour. Structured
  capabilities, not a raw-attribute bag; raw provider attributes are additionally
  retained for attribute-level compliance checks.

Flattening providers entirely would make failure analysis wrong (RDS Multi-AZ and Cloud
SQL HA fail over differently); the capability model is where those differences live
without leaking provider names into `analyse`.

**Resolution state (partial input is the normal case).** Agents call mid-authoring, so
incomplete IaC is expected, not an error. Every node and edge carries a resolution
state — `known` (fully declared), `inferred` (resolved from a reference the engine
could follow), `unresolved` (references a declaration not present in the bundle).
Unresolved elements are never silently dropped: they propagate to analysis as
`not_assessable` rather than a passing or failing result. **Incompleteness must never
become a false negative** — this is a hard design rule, and it feeds the provenance
model directly.

The model is versioned. Every server call against a session produces Version N; diffs
between versions are first-class.

### Provenance (central product principle)

**Every assertion has provenance.** Every field in every response carries a source tag:

```
stated        ← from the workload declaration or user input
derived       ← deterministic computation over the IR (graph traversal, rule engine)
assumed       ← a declared default or distribution the user can override
llm_reasoned  ← LLM contextual judgment, always citing the IR evidence it used
observed      ← measured in a real experiment (validation ladder Rung 1/3)
```

The schema makes untagged assertions unrepresentable.

### Architecture Decision Records and waivers (first-class objects)

Versioning records *what* changed; ADRs record *why*. Each session accumulates ADRs —
decision, reason, trade-offs, controls satisfied, failure modes addressed (linked by
ID) — authored by the engineer or proposed by the agent and confirmed.

**Waivers (schema now, enforcement later).** A finding an engineer has consciously
accepted must not reappear as noise every iteration. A waiver is an evidence object —
who accepted which finding, when, why, against which version, with optional expiry. v1
defines the schema and displays waived findings as `accepted`; enforcement (suppressing
re-raise, expiring waivers on material change) is P2. (PC-20 built the schema and the
scorecard display half; the material-change auto-expiry half remains P2.)

### Workload declaration (required input)

Failure analysis without stated assumptions is guesswork. Callers supply:

```yaml
workload:
  name: payments-api
  criticality: tier1
  data_classification: pci
  regions: [eu-west-2, uksouth]
  compliance_profiles: [pci-dss-4, soc2, cis]

  requirements:
    - {id: rto,          value: 60s,     priority: hard}
    - {id: rpo,          value: 0,        priority: hard}
    - {id: availability, value: 99.95%,   priority: hard}
    - {id: peak_rps,     value: 10000,    priority: hard}
    - {id: cost,         value: minimise, priority: preference, rank: 1}
    - {id: simplicity,   value: prefer,   priority: preference, rank: 2}

  capacity:
    app_node_rps: 3500
```

**Capacity semantics.** Instance *count* answers redundancy (derivable from the graph);
node *capacity* answers whether survivors carry declared peak (only from the
declaration). The engine never treats "3 instances" as "3× capacity" — without declared
per-node capacity the tier is `capacity_unknown` and any capacity finding is
`not_assessable`.

**Requirement priority.** `hard` requirements are constraints — violating one is a
failing finding. `preference` requirements are ranked goals — violating one is a
trade-off, not a failure.

## 5. Functional scope

### 5.1 Ingestion

- **P0:** Terraform / OpenTofu HCL, multi-provider, parsed statically — partial/incomplete
  configs must parse without erroring.
- **The canvas is a second IR producer, symmetric to HCL ingestion (v6, PC-84/86/96).**
  A design authored on the canvas — nodes, containers, capability/sizing/compliance
  selections — feeds the *same unchanged* pipeline as HCL, through the same provenance
  boundary (I2): every canvas-declared value is tagged `stated` at ingestion, not on the
  wire (`ingest/canvas.go`), the same way an HCL attribute has always been. The canvas
  is never a second source of truth and never a way to hand-edit an assessment — see
  §9's non-goals for the precise boundary.
- **Supported HCL subset is defined, not implied.** v1 parses resource/data blocks,
  module calls, variables/locals, static references. Out of scope for v1 (surfaced as
  `unresolved`): `count`/`for_each` expansion, `dynamic` blocks, computed values, complex
  interpolation. `terraform plan` JSON (P1) lifts these limits.
- **Coverage is capability-based, not resource-count based.** The product covers the
  node types above across supported clouds — the golden reference architecture's
  vocabulary (section 10), widened by the Architect Workspace's own expanded AWS
  palette (v6, PC-96) — every added service still declares an honest capability level;
  unsupported/unmodelled resources surface as `not_assessable`.
- **P1:** `terraform plan` JSON. **P2:** Pulumi, CDK-TF, live-account discovery.

### 5.2 Compliance engine

- Control catalog keyed to the IR, not provider resources — one control maps to
  per-cloud checks and multiple frameworks (PCI DSS 4, SOC2, CIS).
- Deterministic checks first; LLM only for contextual judgment, always citing evidence.
- Output is never a bare percentage: applicable / satisfied / partial / unsatisfied /
  not-assessable, each with evidence.
- **IAM policy evaluation (v6, PC-102, P2):** identity/resource/trust-policy evaluation
  with explainable allow/deny decisions, following AWS's own documented logic (explicit
  deny overrides any allow; absence of allow is implicit deny) — never
  "role.permissions includes action → allow." Feeds compliance's own "least privilege"
  evidence and the request trace (§5.4a). Sequenced after the Network & Request Engine
  and Report Generator epics; filed to be tracked, not to jump the queue.

### 5.3 Architecture visualisation

- Rendered from the IR: zones/regions/clouds as containment, redundancy visible.
- Diff view between versions.
- **Not a drawing tool in the sense that matters: the derived assessment view — graph
  rendering, diffs, findings — is never hand-edited to force a result.** The canvas
  (v6, PC-84/86/96) IS a real authoring surface, and it IS how an architect places
  nodes and containers — but it produces IR through the same ingestion boundary as HCL
  (§5.1), and everything downstream of that boundary (the rendered graph, the diff,
  every finding) stays exactly as derived and read-only as it always was. The rule this
  section protects is "you cannot paint a node green," not "you cannot draw."

### 5.4 Failure-mode analysis (static)

```
FM-xxx
  trigger              (what fails)
  affected_components  (graph traversal, deterministic)
  blast_radius         (fraction of capacity / user journeys lost)
  detection            (modeled | declared | unknown | observed)
  existing_mitigation  (redundancy, failover paths present in the graph)
  gap                  (mitigation absent or insufficient vs workload declaration)
  impact               (derived: blast radius × workload criticality)
  likelihood           (assumed | unknown — never fabricated)
  detectability        (from the detection field)
  recoverability       (does a failover path exist; is RTO/RPO feasible)
  recommendation
```

**Severity is not a single number.** Four dimensions reported separately, never
collapsed into one score. **Detection is a typed state**, not a boolean — "unknown"
must never render as "no monitoring exists." **not_assessable applies to resilience
too**, e.g. "RTO feasibility: not_assessable — failover duration is provider-managed and
no assumption was declared."

#### 5.4a Network & Request Engine (v6, PC-97)

Per-request network behaviour, additive to the graph-level reachability above: route
tables, Internet/NAT gateways, Security Groups and NACLs evaluated in real packet
order, and an explainable step-by-step request trace answering "does this specific
request (e.g. ECS task → RDS:5432) actually succeed given routes and firewall rules,"
not just "is there a dependency edge." Security Groups (stateful, allow-only,
ENI-level) and NACLs (stateless, ordered rules, allow/deny, subnet-level) stay separate
abstractions — never collapsed. Every trace step carries provenance and cites the AWS
rule it applied; unmodelled behaviour yields `not_assessable` with a reason, never a
guess. Absorbs PC-79 (SG evaluation for compute-to-data SPOF detection, previously P2)
— PC-112 owns the actual SG evaluation semantics; PC-79's own prefilter becomes a
caller of it. Foundation for traffic flow (§5.8) and configuration-change failure modes
(§5.5).

### 5.5 Failure-mode simulation (the differentiating feature)

**Layer 1 — Structural simulation (P0, the technical centrepiece).** Deterministic
fault injection: kill a node/edge/zone/region/dependency. Recomputes reachability,
surviving capacity vs peak_rps, SPOF (min-cut), RTO/RPO feasibility, cascading
dependency loss. Handles correlated failure natively.

**Layer 2 — Scenario-based reliability analysis (P2, deliberately demoted).**
Originally Monte Carlo over SLA-derived rates; demoted because published SLAs are
contractual thresholds, not failure rates, and infrastructure failures are correlated,
not independent.

**Layer 3 — Discrete-event degraded-state simulation (P2).** SimPy-style, explicitly
conditional on declared assumptions.

**Failure Lab expansion (v6, PC-99):** extends fault injection beyond infrastructure
loss (node/AZ/region, already built via `/simulate`, PC-82/88) to the failure classes
architects actually hit in production — NAT gateway loss, route removal,
security-rule changes, external dependency outages, and combined faults. Configuration
faults ("someone tightened an SG rule") need the Network & Request Engine (§5.4a) to
evaluate and are a genuinely different failure class from "a thing disappears," not a
variant of it. Fault injection mutates the model and re-runs the real engines — it
never just paints nodes red (the same rule PC-89's animation guardrail already
established). Propagation must follow real dependencies and real network paths: NAT
loss breaks private-subnet egress to the internet, not private-subnet-to-RDS traffic —
both halves get their own test, not assumed from one. Every fault result is still
expressed through the existing four-dimension failure-mode schema above (PC-17) and
flows into findings, delta, and the report (§5.10).

### 5.6 Empirical validation ladder

| Rung | Environment | Cost | Validates | Cannot validate |
|---|---|---|---|---|
| 1 | Local topology replica: docker-compose/kind + Toxiproxy on every dependency edge | ~free | App behaviour under dependency failure | Cloud-native semantics, real capacity |
| 2 | Emulated control plane: LocalStack / Azurite | ~free | The tool itself: parser/IR/wiring | Anything performance- or resilience-shaped |
| 3 | Ephemeral real cloud: apply, run FIS/Chaos Studio/Litmus, capture, destroy | pounds/run | Cloud-native failure semantics | Capacity claims at declared peak |

**Emulator boundary rule:** an emulated run must never emit a performance number —
Rung 2 results are typed `functional`. *Emulators answer "is it wired right?",
simulation answers "what should happen given assumptions?", real cloud answers "what
does happen?"*

**The credibility centrepiece:** predicted (Layer 1) vs observed (Rung 1 + one Rung 3
run), published — including at least one prediction the simulation got wrong and the
correction.

### 5.7 Iteration & scoring

- Per-version scorecard, per-dimension — never a composite score.
- Session timeline V1→Vn.
- **Assurance Delta** — `{ improvements[], regressions[], resolved_risks[], new_risks[]
  }`, the payload that makes the agent loop work.

### 5.8 Traffic & Capacity (v6, PC-98)

Lets an architect see how traffic moves through their design: which components carry
each declared user journey, how flow reroutes under failure, and where declared load
exceeds declared capacity. Structural flow (which path carries which journey, where it
reroutes) is derived from the graph and the Network & Request Engine (§5.4a — a
journey the SG/NACL/route engine says fails cannot carry traffic). Quantities (rps per
component, saturation, latency) are computed only from what the architect declared in
the workload/NFR form (§4): journey rps, per-component capacity, service rates. A
missing input yields `not_assessable` with the named missing input — this is the exact
principle that got SLA-based Monte Carlo demoted (§5.5, Layer 2), applied here so it
isn't reintroduced through the back door of a new feature.

### 5.9 Cost Engine (v6, PC-100)

Adds infrastructure cost to every assessment, priced from a **dated, versioned AWS
pricing snapshot** — never a live pricing call during assessment, which would make
identical input produce different output on different days and break NFR-1
(determinism). Each assessment records which snapshot it used; same input + same
snapshot = byte-identical output. The snapshot fetcher holds AWS credentials to call
AWS's pricing APIs, so per the credential boundary (ADR-003) it cannot live in the
assessment engine (P1) — it runs in the credentialed process (P3) or a dedicated job;
`core`'s own cost math is pure (I1) and only multiplies declared sizing by unit prices
read from the snapshot, tagging every resulting figure with provenance. Needs ADR-006
(pricing source and snapshot model) before implementation. List prices are AWS's own
stated informational figures, not a guarantee of what's actually charged — the report
(§5.10) states this plainly wherever a cost figure appears.

### 5.10 Architecture Report Generator (v6, PC-101)

One comprehensive, shareable report per design version. **Principle: the report is a
projection, not a new analysis** — it assembles results that already exist for a stored
version (findings, failure modes, scorecard, delta, SVG diagram, traffic results, cost)
and adds no verdict logic of its own. If a section's source data is `not_assessable`,
the report says so and names what's missing; it never smooths over a gap to look
complete. Target sections: executive summary (hard-requirement pass/fail/not-assessable
counts per dimension, no composite score — the same rule PC-19/83/92 already enforce
on the scorecard); architecture diagram + component inventory; NFR conformance;
compliance (per selected framework, per control, with resource-level evidence);
failure modes; traffic and capacity; cost (with pricing-snapshot date and the list-price
disclaimer above); change since previous version (the Assurance Delta); an assumptions
and provenance appendix.

## 6. Interface design

**Primary: MCP server / HTTP API**, called either directly (the SRE/agent interface,
§3) or by the Architect Workspace's own Simulate/Analyze/Report modes (§3, v6) — both
paths call the identical service functions (§7).

```
POST /sessions/{id}/assess
  body: { iac_bundle, workload.yaml, profiles[] }
  → { version, graph, findings[], failure_modes[], scorecard, diff, assurance_delta }

POST /sessions/{id}/simulate
  body: { version, faults: [{type: zone_loss, target: eu-west-2a}, ...] }
  → { journeys[], capacity, severed_paths[], cascade[], verdict }

POST /sessions/{id}/experiments
  body: { failure_mode_id, target_tooling }
  → { experiment_manifest }
```

Latency budget for `/assess`: deterministic layers <5s; LLM narrative streamed after.

**Secondary: web UI** — as of v6, the Architect Workspace (PC-96/104) IS the web UI,
not a separate, thinner reader over it; the five modes (Design / Simulate / Failure Lab
/ Analyze / Report) share one session/version continuity. **Tertiary (P2): PR mode.**

## 7. System architecture (build view)

**Guiding principle:** one deterministic core; everything else is a consumer of it.
Deployment is **three processes split by real failure domain** — assessment engine
(sync, pure, no credentials), reason worker (async, external LLM, optional), experiment
runner (long-lived, credentialed, the only process holding cloud credentials; as of v6,
also where a pricing-snapshot fetcher runs, §5.9). See the technical design doc §1a and
ADR-003.

```
preflight/
├── core/           # THE product. Zero I/O, zero LLM, zero provider names.
│   ├── ir/         #   two-level graph schema, resolution states, versioning, diffing
│   ├── analyse/    #   SPOF/min-cut, reachability, capacity, compliance rule engine
│   └── simulate/   #   Layer 1 fault injection (P0) · Layers 2–3 scenario/DES (P2)
├── providers/      # data, not code: aws/ azure/ gcp/ mappings + capability model
├── ingest/         # HCL/plan-JSON parsers (partial-tolerant) → core.ir; canvas → core.ir (v6)
├── reason/         # LLM layer: narratives, contextual controls
├── server/         # FastAPI + MCP adapter; session store
├── exporters/      # FIS / Chaos Studio / Litmus / Gremlin manifest generators; FINOS CALM
├── validate/       # validation-ladder harness (Rung 1/2/3)
└── ui/             # Architect Workspace (v6): Design / Simulate / Failure Lab / Analyze / Report
```

**Contracts frozen in week 1, extended when a genuine new producer earns one:** the IR
schema, the provenance enum, `workload.yaml`, the finding/failure-mode JSON, and the
ADR object; `canvas.schema.json` added later (PC-86) on the same standard. Traffic
(PC-98), cost (PC-100), and report (PC-101) results are expected to earn their own
contracts once built — not frozen speculatively here.

## 8. Phasing

**Phase 1 (weeks 1–4): the spine.** Contracts frozen. Terraform parser → IR → graph
render → Layer 1 structural simulation. AWS side of the golden reference architecture
modelled fully. Ship with the IR design note.

**Phase 2 (weeks 5–8): assurance.** Failure-mode enumeration; compliance engine (CIS);
version diffing, scorecard, assurance delta, ADRs; MCP server interface.

**Phase 3 (weeks 9–12): the proof.** Azure side of the golden reference architecture;
experiment exporters; validation ladder in anger; predicted-vs-observed writeup.

**Phase 4 (v6, PC-96–102): the architect platform.** Architect Workspace (canvas-first,
mode-based); Network & Request Engine; Traffic & Capacity; Failure Lab expansion; Cost
Engine; Report Generator; IAM Policy Engine (P2). Sequenced after Phase 1–3's spine is
proven — the same rule §21 (open questions) and CLAUDE.md §21 already state for the
canvas: a wider surface on top of a broken or absent assurance engine is a worse
artifact than a headless engine that actually proves its claim.

## 9. Non-goals

- IaC generation (crowded, off-thesis)
- Measured-performance claims from emulated environments, and unlabelled predictions
  from anywhere
- Live cloud account scanning at launch — including for pricing (§5.9): cost is priced
  from a versioned snapshot, never a live per-assessment API call
- Commercial GTM, pricing (the business kind, not the AWS-cost kind — deliberately the
  same word, deliberately not the same thing), tenancy hardening
- **Freeform diagram editing disconnected from a real IR.** The canvas (v6, PC-84/86/96)
  is an authoring surface that produces IR through the same ingestion boundary as HCL
  (§5.1) — it is not this non-goal's target. What stays out of scope, precisely: hand-
  editing the *derived* assessment view (the rendered graph, a diff, a finding) to make
  it say something the engine didn't compute. Design your architecture on the canvas
  freely; you cannot draw a passing verdict.
- Rubric scoring or composite scores of any kind, on the canvas or anywhere else (PC-96's
  own explicit scope exclusion, carrying forward the same rule §5.7/PC-19/83/92 already
  enforce)

## 10. Golden reference architecture (the demonstration surface)

One payments workload, modelled exceptionally, rather than broad shallow coverage:

```
Payments API — tier1, PCI, 99.95%, RTO 60s, RPO 0

AWS                          Azure (equivalent capabilities)
├── Route53        (dns)     ├── Azure DNS
├── WAF                      ├── Front Door
├── ALB      (load_balancer) ├── Application Gateway
├── EKS  (container_workload)├── AKS
├── RDS   (managed_database) ├── Azure SQL
├── ElastiCache    (cache)   ├── Azure Cache for Redis
├── SQS            (queue)   ├── Service Bus
└── IAM         (identity)   └── Entra / RBAC
```

Six failure scenarios are made exceptional: **AZ loss, database failure, queue failure,
identity/credential failure, region loss, external-dependency (payment rail) outage.**

## 11. Success criteria (portfolio framing)

1. **The agent-iteration acceptance test (primary).** The golden reference architecture
   ships deliberately broken. An AI agent, consuming only the assurance delta, must
   drive it to a target state within bounded iterations — a runnable pass/fail test.
2. A recorded end-to-end demo across both clouds, with a simulated region loss verified
   by an exported chaos experiment.
3. Predicted vs observed writeup for ≥3 of the six scenarios, including at least one
   miss and its correction.
4. The IR design note strong enough to stand alone as a technical artifact.
5. A senior SRE reading the repo concludes the author understands failure, evidence,
   and the limits of LLM judgment.

## 12. Open questions

- How partial is partial: MVG floor now defined (≥1 entry point + ≥1 stateful node →
  otherwise `insufficient_model`); thresholds to be tuned.
- Cross-cloud journeys (DNS failover AWS→Azure) — Phase 3 or explicitly defer?

*Resolved in v4:* capacity semantics. *Resolved in v5:* IR schema (custom + CALM
exporter, one-way and lossy), MVG floor. *Resolved in v6 (PC-103):* primary user
(architect on canvas, not SRE+agent — agent loop unchanged as a second interface);
functional scope for traffic flow, expanded failure lab, cost, report, IAM; the canvas
non-goal narrowed to what it actually protects (hand-editing the derived view, not
authoring on the canvas at all).
