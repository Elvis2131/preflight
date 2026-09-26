# Preflight Technical Design

**Source of truth:** Confluence, `Preflight Technical Design`, page ID `2850839`
(`https://elartey.atlassian.net/wiki/spaces/~5bbb38ce5a05242b81c6e0f0/pages/2850839`).
This file is a synced local copy, created by PC-103 (previously many tickets — PC-6,
PC-7, PC-9, PC-10, PC-77 among them, per their own comments — cited this path without
the file existing on disk; ADR-003's own "Provenance" note records the same gap for the
period before this file existed). If this file and the Confluence page ever disagree,
Confluence is authoritative.

**Companion to:** `architecture-assurance-prd.md` (v6, architect-first pivot, PC-103)
**Owner:** Elvis / Reppl.sh
**Status:** Design v6 (PC-103) — v2 added process topology and the first three ADRs;
v3 supersedes the language decision (Go replaces Python, ADR-004); v4 supersedes the LLM
provider (NVIDIA's free API via direct `net/http` replaces Claude/`anthropic-sdk-go`,
ADR-005); v5 added a sixth frozen contract (`canvas.schema.json`, PC-86) with its
provenance boundary at ingestion rather than on the wire; **v6 records the
architect-first pivot's own topology/contract consequences** — the pricing-snapshot
fetcher's placement (§1a) and a forward note on the new engines' anticipated contracts
(§5) — without freezing anything speculatively (PC-103's own scope: record what
changes, don't design ahead of the epics that will actually build it); **v7 adds a
seventh frozen contract (`report.schema.json`, PC-120)** — see §5's own updated note:
this doc's own v6 forward-note guess ("most likely no new contract for the report
itself... a pure projection") did not hold once PC-120 was actually specified — the
Card explicitly calls for a versioned `report.schema.json`. "Projection" describes the
report's own VERDICT logic (it invents none — every value traces to an existing
engine's own output), not whether its response shape needs a schema; a real,
structured wire type spanning six other engines' outputs still needs one, the same
"a genuine new producer earns its own contract" standard `canvas.schema.json` set.
**Scope of this doc:** end-to-end flows, non-functional requirements, and the
technology selection those NFRs force. Product rationale lives in the PRD and is not
repeated.

**Method note:** technologies here are *derived from* the NFRs in section 3, not chosen
first and justified after. Section 4 shows the derivation. Where an NFR does not
constrain a choice, the choice is deliberately boring.

---

## 1. Architectural invariants

Five rules the implementation may not violate. Everything downstream is a consequence.

| # | Invariant | Enforcement mechanism |
|---|---|---|
| I1 | `core` is pure: no I/O, no network, no LLM, no provider names, no clock, no randomness without an injected seed | `internal/` package placement (compiler-enforced, unconditional) plus `golangci-lint` for defense in depth; `core` cannot import `ingest`, `reason`, `server`, `providers` |
| I2 | Every assertion carries provenance | Provenance is a required field on the base value type; untagged values are unconstructable |
| I3 | `reason` (LLM) annotates, never mutates | `reason` receives a frozen assessment and returns a separate annotation object; merge is additive and provenance-tagged `llm_reasoned` |
| I4 | Incompleteness surfaces as `not_assessable`, never as pass or fail | Tri-state resolution on every node/edge; analysis returns `Assessment[T] = Assessed(T) \| NotAssessable(reason)` |
| I5 | Evidence tier cannot be laundered | Rung-2 (emulated) results are typed `functional` and structurally rejected by performance/resilience fields |

If a future feature requires breaking one of these, it does not ship. This includes
every v6 epic (PC-96–102): none of them are exempted from I1–I5, and none has asked to
be — see the PRD's own v6 non-goals (§9) for the specific, narrowed rule that keeps the
canvas from becoming an exception to I4/I2 in practice, not just in principle.

---

## 1a. Process topology — bulkheads by failure domain, not by module

The system is **three processes, split along real failure domains**, not a flat
monolith and not a service-per-module decomposition. The split is justified by what can
actually fail independently:

```
┌──────────────────────────────────────────────────────┐
│ P1 · Assessment engine            (sync, pure, fast) │
│   ingest → core (analyse, simulate) → API/MCP        │
│   No credentials. No external calls. No LLM.         │
│   Fails only by returning an error; nothing to       │
│   cascade from. NFR-6 budget lives entirely here.    │
└──────────────────┬───────────────────────────────────┘
                   │ frozen assessment (already returned to caller)
                   ▼
┌──────────────────────────────────────────────────────┐
│ P2 · Reason worker           (async, external, opt.) │
│   NVIDIA API calls (net/http, no SDK), narrative only │
│   Failure domain: an external network dependency     │
│   with unbounded latency. Can be down; the product   │
│   degrades, never fails.                    [NFR-11] │
└──────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────┐
│ P3 · Experiment runner    (long-lived, credentialed) │
│   validate/ Rung 1 + Rung 3: terraform apply, FIS/   │
│   Chaos Studio/Litmus execution, capture, destroy    │
│   v6 (PC-100): also the pricing-snapshot fetcher —   │
│   same credential boundary, same reason (below)      │
│   Failure domain: minutes-long jobs, cloud creds,    │
│   different security posture and lifecycle entirely. │
│   THE ONLY PROCESS THAT EVER HOLDS CREDENTIALS.      │
└──────────────────────────────────────────────────────┘
```

**Why this split and not more.** Bulkheads isolate failure domains; a bulkhead around
something that cannot partially fail is cost without benefit. `core` is pure functions
over an in-memory graph completing in milliseconds — no resource pool to exhaust, no
downstream to cascade from, no partial-failure state. Wrapping `analyse`, `simulate`,
and `compliance` in separate services would add network hops *inside* the NFR-6 budget
and introduce new failure modes (the hops themselves) to protect against failure modes
that do not exist. The three domains above are different: an external API with
unbounded latency, and a credentialed long-running job, genuinely can fail in ways the
assessment path must survive.

**Why P3 is non-negotiable.** Cloud credentials must not share an address space with
the request-handling API (NFR-15). This is a security boundary, not a scaling one.

**v6 addition: the pricing-snapshot fetcher (PC-100) belongs in P3, for the identical
reason.** It holds AWS credentials (the Price List Bulk/Query API requires an IAM
identity with pricing permissions — verified against AWS Billing docs before PC-100's
own Card was written), so per NFR-15/ADR-003 it cannot live in P1. `core`'s own cost
math stays pure (I1): it reads a dated snapshot the fetcher already produced and
multiplies declared sizing by unit prices — the fetcher's credentialed fetch and the
engine's pure computation are two different failure domains for exactly the same reason
P1/P3 already are. Needs ADR-006 (pricing source and snapshot model) before
implementation; not written as part of this PC-103 update, since PC-103's own scope is
recording the topology consequence, not designing the snapshot format ahead of PC-100.

**Enforcement, resolved (PC-10):** `cmd/runnerd/internal/creds` makes "P3 is the only
credential holder" a compiler-enforced boundary, the same `internal/` mechanism I1
already uses — see that package's own doc comment and `core/boundary_test.go`'s sibling
test for the proof. Nothing lives there yet (no real credential-loading code exists —
Rung 2/3 of `validate/`, PC-25, and the PC-100 pricing fetcher above, aren't built); the
boundary is structural before there is content inside it worth protecting, the same
sequencing this section's own topology was stood up with before P1/P2's real logic
existed.

**Within P1, boundaries are enforced by CI, not by network.** `internal/` package
placement (I1) makes `core`'s purity structural. If a component ever needs independent
scaling or independent failure isolation, the package boundaries are the extraction
seams — but extraction is justified by an observed need, never anticipated.

---

## 2. End-to-end flows

### 2.1 The assurance loop (primary flow)

```
Agent/engineer                Server                    Core (pure)
     │                          │                            │
     │─ POST /assess ──────────▶│                            │
     │  {iac_bundle,            │                            │
     │   workload.yaml}         │                            │
     │                          │─ parse (ingest) ──────────▶│
     │                          │   HCL → provider mapping   │
     │                          │   → two-level IR           │
     │                          │   resolution states set    │
     │                          │                            │
     │                          │◀─ IR vN ───────────────────│
     │                          │                            │
     │                          │─ analyse ─────────────────▶│
     │                          │   graph rules              │
     │                          │   compliance rules          │
     │                          │   failure enumeration      │
     │                          │   Layer-1 simulation       │
     │                          │   diff vs vN-1             │
     │                          │◀─ findings, FMs, delta ────│
     │                          │                            │
     │◀─ deterministic payload ─│  (target < 5s, I1 pure)    │
     │  {version, graph,        │                            │
     │   findings, failure_     │                            │
     │   modes, scorecard,      │                            │
     │   assurance_delta}       │                            │
     │                          │                            │
     │                          │─ reason (async) ──────────▶ LLM
     │◀═ SSE: narratives ═══════│   annotations only (I3)
     │   provenance=llm_reasoned│
     │                          │
     │─ modifies IaC ──────────▶│
     │─ POST /assess (vN+1) ───▶│   ... loop
```

**Key property:** the deterministic payload is complete and actionable on its own. The
LLM stream is additive. An agent that ignores the SSE channel entirely still passes the
acceptance test. As of v6, the Architect Workspace's canvas-authored path (PC-86)
enters this exact same flow at the same point — a canvas design produces an IR the same
way `ingest` does, so everything from "IR vN" onward in the diagram above is identical
regardless of which producer supplied it.

### 2.2 Ingestion detail (where correctness is won or lost)

```
IaC bundle
   │
   ├─ HCL parse (hashicorp/hcl) ──▶ raw blocks
   │        │
   │        └─ unsupported construct (count/for_each/dynamic)
   │             → node emitted with resolution_state=unresolved
   │               + reason; never dropped              [I4]
   │
   ├─ reference resolution
   │        known      → target block found in bundle
   │        inferred   → resolved through a module/variable indirection
   │        unresolved → reference to absent declaration
   │
   ├─ provider mapping (providers/aws/*.yaml)
   │        aws_db_instance → canonical: managed_database
   │                        → capabilities: replication_mode,
   │                          failover_mechanism, multi_az, encryption
   │        unmapped resource → not_assessable node        [I4]
   │
   ├─ Minimum Viable Graph check
   │        < 1 entry point OR < 1 stateful node
   │           → 200 with {status: "insufficient_model",
   │                       missing: [...], guidance: [...]}
   │           → NOT a wall of not_assessable findings
   │
   └─ two-level IR vN (canonical + provider capability), all fields
      provenance-tagged, hashed for content-addressed versioning
```

### 2.3 Simulation flow (Layer 1)

```
IR vN + fault set {zone_loss: eu-west-2a}
   │
   ├─ mark contained nodes failed (containment closure — correlated
   │   failure handled natively, not as independent events)
   ├─ transitive dependency closure over depends_on
   ├─ per-journey reachability (declared journeys, BFS on surviving graph)
   ├─ capacity check:
   │     declared per-node capacity present? → survivors × capacity vs peak_rps
   │     absent?                             → capacity_unknown → not_assessable  [I4]
   ├─ SPOF: min-cut over tier-1 journeys
   └─ RTO/RPO feasibility: read failover_mechanism + replication_mode
         from provider capability model; unknown → not_assessable
   │
   └─▶ SimulationResult, every field tagged derived|stated|assumed
```

**v6 (PC-97, PC-98):** the Network & Request Engine sits ahead of "per-journey
reachability" above for any journey the architect wants evaluated at request-granularity
— a journey the SG/NACL/route engine says fails cannot carry traffic, which is exactly
the dependency PC-98's own Card names. Traffic & Capacity's flow/bottleneck computation
is a consumer of this same simulation flow, not a parallel one.

### 2.4 Validation ladder flow (Phase 3)

```
FailureMode FM-003 (region loss)
   │
   ├─ minimum validating rung = 3 (cloud-native semantics required)
   │
   ├─ Rung 1 (Toxiproxy/kind): app-behaviour classes only
   ├─ Rung 2 (LocalStack):     parser/wiring round-trip only,
   │                           result typed `functional`          [I5]
   └─ Rung 3 (ephemeral cloud):
          terraform apply → FIS experiment → capture → destroy
          → ObservedResult{provenance: observed, rung: 3}
   │
   └─▶ predicted-vs-observed diff → checked in as regression fixture
```

(PC-24 built Rung 1 against exactly this flow, narrowed to one dependency edge — see
that ticket's own scope note for why, and `validate/rung1.go`'s doc comment for the
real, hand-verified fault signal it produces.)

---

## 3. Non-functional requirements

NFRs are numbered and referenced by the technology decisions in section 4. Each has a
measurable target and a verification method — an NFR that cannot be tested is a wish.

### 3.1 Correctness and determinism

| ID | Requirement | Target | Verification |
|---|---|---|---|
| NFR-1 | **Bit-identical determinism.** Same IR + same rules → byte-identical assessment, across runs and machines | 100% | Golden-file snapshot tests; same fixture run 100× in CI must hash identically |
| NFR-2 | **No untagged assertions.** Every field in every response carries provenance | 100%, structurally enforced | Schema validation on every response in CI; property test attempts to construct untagged value and must fail to compile/validate |
| NFR-3 | **No false negatives from incompleteness.** Missing input never yields a passing or failing verdict | 0 violations | Mutation testing: delete each resource from the golden fixture, assert affected findings become `not_assessable`, never flip to pass/fail |
| NFR-4 | **Evidence-tier integrity.** No emulated result populates a performance or resilience field | 0 violations | Type-level; negative test asserting rejection |
| NFR-5 | **LLM cannot alter verdicts.** Deterministic output identical with `reason` enabled and disabled | 100% | Differential test: run assessment with LLM stubbed vs live, deterministic payload must be identical |

### 3.2 Performance

| ID | Requirement | Target | Rationale |
|---|---|---|---|
| NFR-6 | `/assess` deterministic payload latency, golden reference architecture (~40 nodes) | p95 < 5s, p99 < 8s | It is an authoring-loop call; beyond ~5s engineers and agents stop using it mid-edit |
| NFR-7 | `/simulate` single fault set | p95 < 1s | Interactive "what if" exploration |
| NFR-8 | Parse + IR construction | < 1.5s for 200 resources | Leaves budget for analysis inside NFR-6 |
| NFR-9 | LLM narrative first token | < 3s after deterministic payload | Streamed, so never blocks NFR-6 |
| NFR-10 | Graph algorithm complexity ceiling | Min-cut ≤ O(V·E) on ≤500-node graphs | Bounds worst case for realistic estates without premature optimisation |

### 3.3 Reliability and operability

| ID | Requirement | Target |
|---|---|---|
| NFR-11 | LLM provider unavailable → deterministic assessment still returns, degraded flag set | 100% availability of core path independent of LLM |
| NFR-12 | Session state durability across restart | No loss of approved versions/ADRs/waivers |
| NFR-13 | Structured logs and traces with session/version correlation | Every assessment traceable end to end |
| NFR-14 | Self-hosted, homelab-runnable, no managed-service dependency beyond the LLM API | Single `docker compose up` for local dev |

### 3.4 Security and trust

| ID | Requirement | Target |
|---|---|---|
| NFR-15 | **No cloud credentials required or accepted** for the P0 path | Static analysis only; credentials only in `validate/` Rung 3 (sandbox-scoped) and, as of v6, the PC-100 pricing-snapshot fetcher — both live in P3, never P1 |
| NFR-16 | IaC content never sent to the LLM wholesale — only IR-derived, minimised context | Enforced by a context-builder that takes IR fields, never raw files |
| NFR-17 | Prompt-injection resistance: IaC comments/strings cannot alter LLM instructions | Adversarial fixture with injected instructions in a comment; assert verdicts unchanged (relies on I3) |
| NFR-18 | Secrets detected in IaC are redacted before any logging or LLM call | 0 leaks in CI scan |

### 3.5 Maintainability and evolvability

| ID | Requirement | Target |
|---|---|---|
| NFR-19 | **Adding a cloud provider requires no change to** `core` | New provider = new `providers/<cloud>/` data folder + mapping tests only |
| NFR-20 | Frozen contracts are versioned and validated | JSON Schema for IR, provenance, finding, workload, ADR; breaking change requires schema version bump |
| NFR-21 | Test coverage on `core` | ≥90% line, 100% on simulation and resolution-state paths |
| NFR-22 | Contract-level CI gate | `internal/` boundaries (I1), schema validation (NFR-2), determinism hash (NFR-1) all block merge |

### 3.6 Scale boundaries (explicit non-requirements)

Stating these prevents over-engineering: **not** multi-tenant, **not** horizontally
scaled, **not** high-availability, **not** handling >500-node estates, **not**
sub-second at 10k nodes. Single-instance, single-user, homelab. Any technology chosen
for scale beyond this is a wrong choice for this build. (v6's wider palette and new
engines widen functional surface, not this scale envelope — PC-96–102 don't relax
this section.)

---

## 4. Technology selection, derived from NFRs

Each decision names the NFRs that force it and the rejected alternative.

### 4.1 Core language — Go 1.26+

**Driven by:** NFR-19 (provider data over code), NFR-21 (testability), NFR-10 (graph
algorithms), NFR-14 (single-binary deployment). Superseded from Python per **ADR-004**.

Go's `internal/` package convention gives I1 a compile-time, unconditional guarantee.
`hashicorp/hcl` is the reference implementation of Terraform's own parser. The official
MCP Go SDK (`github.com/modelcontextprotocol/go-sdk`) covers the agent-facing interface.

**Consequence to manage:** determinism (NFR-1) still requires discipline in Go — map
iteration order is deliberately randomized. Sort before any traversal that feeds output
ordering. **Accepted gap:** NetworkX's built-in min-cut has no Go equivalent; PC-14
implements a max-flow-based cut by hand.

### 4.2 Contract enforcement — Go structs, `go-playground/validator`, generated JSON Schema

**Driven by:** NFR-2, NFR-20, NFR-4. Struct tags carry the shape, `go-playground/
validator` enforces requiredness/constraints at runtime, `invopop/jsonschema` generates
the exported schema from the same structs so contract and code cannot drift apart.

### 4.3 Graph engine — `gonum/graph`, with a hand-implemented min-cut

**Driven by:** NFR-10, NFR-1, NFR-14. `gonum/graph` provides connectivity/traversal
in-process; SPOF detection (PC-14) implements a max-flow-based cut directly against the
IR graph, wrapped behind `core/ir/graph.go` so the IR — not `gonum` — is the contract.

### 4.4 HCL ingestion — `hashicorp/hcl`, the reference parser

**Driven by:** NFR-8, NFR-3, the PRD's defined HCL subset. Constructs outside the
subset are detected and emitted as `unresolved` nodes with a reason (I4) rather than
raising.

### 4.5 Provider mappings — declarative YAML under `providers/`

**Driven by:** NFR-19, I1. Mappings are data, loaded and validated against a mapping
schema at startup; `provenance` is declared *in the mapping*, so provider-behaviour
assumptions are tagged `assumed` rather than masquerading as `stated`/`derived`.

### 4.6 Compliance rule engine — internal Go registry over the IR, not OPA (for v1)

**Driven by:** NFR-1, NFR-2, NFR-6, NFR-21. Rules are Go functions registered against a
control ID via an `init()`-time registry, each returning `Assessment[Finding]` with
evidence references. **This is a v1 decision, revisited if the control catalog exceeds
~100 controls** — the v6 IAM engine (PC-102) and NACL engine (PC-113) both add to that
catalog; the trigger is unchanged by their addition, but they move the count closer to
it, worth tracking.

### 4.7 API layer — `net/http` + `chi` for SSE, official MCP Go SDK

**Driven by:** NFR-6/NFR-9, NFR-13, primary-user design. `github.com/
modelcontextprotocol/go-sdk` wraps the same service layer HTTP routes call, one
implementation, one set of tests — unchanged by the v6 pivot: the Architect Workspace's
own Simulate/Analyze/Report modes call this exact API, not a separate one (PRD §6).

### 4.8 Persistence — SQLite (WAL, relational) with a repository interface

**Driven by:** NFR-12, NFR-14, NFR-1, NFR-3.6. Sessions, versions, findings, ADRs, and
waivers are stored with indexed keys and JSON payloads. Content-addressed version
hashing makes diffing cheap and NFR-1 externally verifiable. **v6 note:** PC-100's
pricing snapshots are a candidate for the same store (dated, versioned, referenced by
assessment) — not designed here; PC-100's own ADR-006 owns that decision.

### 4.9 LLM integration — NVIDIA API catalog via direct `net/http`, no SDK, strict isolation

**Driven by:** NFR-5, NFR-16, NFR-17, NFR-11. Superseded from Claude/`anthropic-sdk-go`
per **ADR-005**. `reason/` calls NVIDIA's OpenAI-compatible endpoint directly; no SDK.

### 4.10 Visualisation — server-rendered SVG from the IR

**Driven by:** NFR-1, the PRD's "derived, never hand-edited" integrity claim (narrowed
in v6 to precisely what it protects — PRD §5.3/§9), NFR-14. Graph layout computed
server-side (Graphviz) and emitted as SVG with stable node IDs (PC-81).

### 4.11 FINOS CALM exporter — interchange without coupling

**Driven by:** interchange credibility with regulated-sector stakeholders, NFR-19,
NFR-20. `exporters/calm.go` (PC-27) projects the frozen IR into valid CALM JSON,
validated in CI against the vendored official CALM JSON Schema. **Stated limitation:**
the export is one-way and lossy — provenance tags and resolution states have no
first-class CALM equivalent; stated plainly inside the exported document's own
metadata, not only here.

### 4.12 Validation harness — Toxiproxy, LocalStack, Terraform, FIS/Litmus

**Driven by:** NFR-4, NFR-15, PRD ladder.
- **Rung 1:** `kind`/docker-compose topology replica, Toxiproxy on the dependency edges
  that matter (PC-24 narrowed this to app-tier→database first, "start narrow," not all
  edges at once). Results typed `functional` (I5) — never `behavioural` as an
  unenforced label; the schema enforcement is what makes the tier real, not the name.
- **Rung 2:** LocalStack round-trip. Results typed `functional` — schema-rejected from
  performance/resilience fields (NFR-4, I5).
- **Rung 3:** ephemeral `terraform apply` into a sandbox subscription, FIS/Chaos
  Studio/Litmus experiment, capture, `destroy`. Results typed `observed`, the only tier
  permitted to populate measured fields. Credentials exist only here (and, v6, in the
  pricing fetcher, §1a), scoped to a sandbox account, never in the P0 path (NFR-15).

### 4.13 CI and enforcement — the invariants must be executable

**Driven by:** NFR-22, NFR-1, NFR-2, NFR-19.

| Gate | Tool | Enforces |
|---|---|---|
| Import boundaries | `internal/` package placement + `golangci-lint` | I1 — `core` cannot import `ingest`/`reason`/`server`/`providers`; P1/P2 cannot import P3's credential code (PC-10) |
| Determinism | `go test` + hash comparison, sorted traversal enforced by lint rule | NFR-1 |
| Schema validity | `go-playground/validator` + generated JSON Schema validation of every response fixture | NFR-2, NFR-20 |
| Incompleteness safety | mutation tests (delete-a-resource sweep) | NFR-3 |
| LLM independence | differential run, stubbed vs live | NFR-5 |
| Provider additivity | new-provider fixture must pass without core diff | NFR-19 |
| Coverage | `go test -cover`, ≥90% core | NFR-21 |
| Acceptance | agent-iteration test (section 6) | PRD success criterion 1 |

**Stack summary:** Go 1.26+, `go-playground/validator` + `invopop/jsonschema`,
`gonum/graph` + hand-implemented min-cut, `hashicorp/hcl`, `net/http` + `chi` + official
MCP Go SDK, SQLite/WAL, NVIDIA API catalog via direct `net/http` (ADR-005), Graphviz→SVG,
Docker/kind + Toxiproxy + LocalStack + Terraform, `go test` + `golangci-lint` +
`internal/` boundaries, all self-hosted. Language decision: **ADR-004**. LLM provider
decision: **ADR-005**.

---

## 5. Data contracts (frozen week 1, extended when a genuine new producer earns one)

Six schemas in `contracts/`, JSON Schema generated from the same Go structs the code
validates against (§4.2), each versioned independently.

| Contract | File | Governs |
|---|---|---|
| IR | `ir.schema.json` | Two-level model, resolution states, per-field provenance, version hash |
| Provenance | `provenance.schema.json` | The five-value enum and the `Tagged`/`Assessment` wrappers |
| Workload | `workload.schema.json` | Requirements with priority, declared capacity, compliance profiles |
| Finding & failure mode | `finding.schema.json` | Four-dimension failure model, typed detection, evidence refs, not_assessable |
| ADR & waiver | `adr.schema.json` | Decision records, risk acceptance, expiry |
| Canvas | `canvas.schema.json` | Wire format for the canvas UI's nodes/edges/capability (PC-84/85/86) — the second IR producer, added once the canvas epic reached the point of needing one |

Changes to these require a schema version bump and a migration note. Everything else
in the codebase may churn freely — that asymmetry is the point.

**The canvas contract's provenance boundary sits at ingestion, not on the wire** — a
deliberate exception worth naming, not an oversight. The other five contracts either
carry provenance as part of their own shape (IR, Finding) or exist specifically to
define the provenance model itself. `canvas.schema.json` doesn't: `CanvasNode.
capability` is a plain, untyped string map at the wire level, with no provenance field
enforced by the schema. I2 still holds — every value gets tagged `stated` — but the
tagging happens in `ingest/canvas.go` when the wire document is converted into
`core.IR`, not before.

**v6 forward note (PC-103's own scope: record the consequence, don't design ahead of
the epics that will build it):** the Network & Request Engine (PC-97) and Traffic &
Capacity (PC-98)'s own contracts are still not frozen here. Cost (PC-100) has since
been built (PC-116/117/118) WITHOUT earning a dedicated contract of its own — its
fields extended `Scorecard`/`Fault` instead, plain additive Go types outside the
six-then-seven frozen contracts, the same "not every new producer needs one" latitude
`canvas.schema.json`'s own precedent always implied, not a rule that every epic must
freeze something. **Report (PC-101/PC-120) has now been built, and DID earn its own
contract** (`report.schema.json`, §8's count now seven) — this doc's own earlier guess
here ("most likely no new contract for the report itself... a pure projection of
results the other five/six contracts already govern") did not hold: "projection"
describes PC-120's own VERDICT logic (it invents none), not its wire shape — see this
section's own v7 history note above. Freezing Capacity's contract ahead of the epic
that needs it would be exactly the speculative-generality this project's own
scenario-driven minimalism rule (PRD §4) warns against.

---

## 6. Testing strategy

Four tiers, each mapped to what it protects:

1. **Unit (core, ≥90%)** — pure functions over fixtures; every simulation and
   resolution-state path at 100% (NFR-21).
2. **Golden-file snapshots** — reference Terraform bundles → expected IR → expected
   findings/simulations, byte-compared (NFR-1). These are the regression backbone.
3. **Property/mutation tests** — delete-a-resource sweep asserting `not_assessable`
   propagation (NFR-3); untagged-value construction attempts (NFR-2); adversarial
   injection fixtures (NFR-17).
4. **Acceptance — the agent-iteration test.** The golden reference architecture ships
   deliberately broken; a scripted agent loop consuming only the assurance delta must
   reach the target state (zero tier-1 SPOFs, hard requirements satisfied) within N
   iterations. **This is the test that fails loudly if the product's output isn't good
   enough to act on** — it is the primary success criterion, not a smoke test.

Predicted-vs-observed results from Rung 1/3 runs are checked in as fixtures, so a later
engine change that would have broken a validated prediction fails CI.

---

## 7. Risks and mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Go map-iteration nondeterminism breaks NFR-1 | Undermines core integrity claim | Sorted traversal enforced in `core` by lint rule, determinism hash gate in CI (ADR-004) |
| Static HCL parsing too weak for realistic Terraform | Demo looks toy | Defined subset stated up front; plan-JSON (P1) is the escape hatch |
| Provider capability modelling balloons | IR drifts to universal cloud model | Scenario-driven minimalism rule: no field without a golden scenario needing it |
| LLM narrative contradicts deterministic findings | Trust damage | `reason` evaluated against goldens; contradiction is a test failure |
| 12 weeks is optimistic solo, alongside employment | Phase 3/4 unfinished | Phase 1 alone is a publishable artifact; each phase ends shippable |
| Rung 3 cloud spend | Cost creep | Ephemeral only, destroy in the same run, sandbox account with a budget alarm set before anything runs |
| **v6:** wider palette / new engines (PC-96–102) balloon scope before Phase 1–3's spine is proven | A wide, shallow platform on top of an unproven core | Explicitly sequenced as Phase 4, after Phase 1–3 (PRD §8); each new epic's own Card states its own dependency on already-built engines rather than starting fresh |

---

## 8. What Phase 1 actually delivers

The concrete week-4 definition of done, so progress is unambiguous:

- `contracts/` — five frozen JSON Schemas (as of Phase 1; a sixth, `canvas.schema.
  json`, and a seventh, `report.schema.json` (PC-120), were added later — see §5)
- `core/ir` — two-level model, resolution states, provenance wrappers,
  content-addressed versioning, diffing
- `providers/aws` — mappings for the eight golden-architecture capability types
- `ingest` — HCL parser over the defined subset, MVG check, unresolved-node emission
- `core/analyse` + `core/simulate` — reachability, SPOF/min-cut, capacity
  (declared-only), Layer-1 fault injection
- Golden reference architecture (AWS side) as real Terraform, plus its golden-file
  fixtures
- CI with all NFR gates from 4.13 wired
- **The IR design note** — publishable standalone (PRD success criterion 4)

No API, no LLM, no UI in Phase 1. Those are Phase 2. The spine has to be right first,
because everything else — including the v6 architect platform — consumes it.
