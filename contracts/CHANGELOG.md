# Contract changelog

Design §5: "Changes to these require a schema version bump and a migration note.
Everything else in the codebase may churn freely — that asymmetry is the point."

Every entry below states: which schema changed, the version bump, and what a caller
holding a document against the old version needs to do.

Each generated `contracts/*.schema.json` carries its own version in `x-schema-version`,
stamped by `cmd/gen-contracts` — check that field against this file, not the other way
around, since a schema file is regenerated output, not hand-edited.

## canvas.schema.json 1.4.0 — 2026-10-01 (PC-105: CanvasNode.availability_zone and cidr_block added)

**Additive change to `canvas.schema.json` only.** `CanvasNode` gains two optional
fields, `availability_zone` (a subnet's one zone — a subnet cannot span zones) and
`cidr_block` (a VPC's or subnet's IPv4 range), stamped by `ingest/canvas.go` onto
`RawAttributes["availability_zone"]` / `["cidr_block"]`: the same keys the Terraform path
already uses, so both producers yield one IR shape. Region ⊃ VPC ⊃ AZ ⊃ Subnet ⊃ resource
nesting is carried by existing `contained_in` edges plus these attributes — an AZ is an
attribute of a subnet (as in Terraform), not a node. A document against 1.3.0 needs no
change. Behaviour change worth knowing: `POST /sessions/{id}/canvas` now rejects invalid
placement with HTTP 422 `error_code: invalid_placement` (see `core/canvas_placement.go`).

## workload.schema.json 1.4.0 — 2026-10-01 (PC-131: DeclaredJourney.fallback added)

**Additive change to `workload.schema.json` only** — the other six schemas are
untouched and remain at their current versions.

`DeclaredJourney` gains one new, optional field: `fallback` (`description`, `path`) —
a journey's DECLARED degraded mode, e.g. a queue buffering payment requests while the
payment rail is down. `path` uses the same vocabulary as `DeclaredJourney.path` and is
evaluated by the same flow engine under the same fault. A journey with no `fallback`
that does not flow simply fails: graceful degradation is never assumed (PC-131's own
rule). A caller holding a document against 1.3.0 needs to do nothing — the field is
optional and absence means "no fallback declared", exactly the prior behaviour.

## report.schema.json 1.4.0 — 2026-10-01 (PC-131: JourneyFlowResult.Degraded/DegradedVia added)

**Additive change to `report.schema.json` only** (consequential to workload 1.4.0
above: the report embeds journeys and per-journey flow results). `JourneyFlowResult`
gains two optional fields, `Degraded` and `DegradedVia`: set only when the primary
path does not flow but the journey's declared `fallback` does. Both are `omitempty`,
so a document from before this change is still valid and a reader should treat absence
as `Degraded: false`.

## canvas.schema.json 1.3.0 — 2026-09-26 (PC-137: CanvasNode.security_group_rules added)

**Additive change to `canvas.schema.json` only** — the other six schemas are
untouched and remain at their current versions.

Found during PC-136: once canvas nodes resolved their capability level, every
canvas-built journey moved past the capability gate and immediately stopped at the
Security Group check, because the canvas had no way to author SG rules at all —
Terraform-loaded designs work because `ingest/securitygroups.go` reads real
`aws_security_group`/`aws_security_group_rule` resources; the canvas had no
equivalent. `CanvasNode` gains `security_group_rules`, a list of the new
`CanvasSecurityGroupRule` (`direction`, `protocol`, `from_port`, `to_port`,
`cidr_blocks`, `source_security_group`) — a structured list, not a string map like
`capability`/`sizing`, since one rule is itself a small record (the same shape
`ingest/securitygroups.go`'s own `normalizeSGRule` already produces, so
`ingest/canvas.go` can stamp it onto `RawAttributes["security_group_rules"]`
unchanged and `core.SecurityGroupProfile` reads it back identically regardless of
which producer built the IR). A component attaches to a security group via the
existing `depends_on` edge type — no new edge type, no new attachment mechanism,
exactly mirroring how the Terraform path already represents SG attachment.

Alongside this contract change, `core/trace.go`'s `sg_dest_ingress` step was
corrected: a resource with genuinely zero attached security groups (no `depends_on`
edge to any node carrying `security_group_rules` at all) now yields `not_assessable`
— missing data, not a computed implicit deny — matching `nacl_check`'s own existing
convention for an unresolvable NACL. A real SG that IS attached but whose own
authored rules simply don't match stays exactly what it always was: a genuine,
computed deny.

## report.schema.json 1.3.0 — 2026-09-26 (PC-125/126: parallel-path fields added)

**Additive change to `report.schema.json` only** — the other six schemas are
untouched and remain at their current versions.

Filed after PC-136 surfaced that PC-125/126's own stated "parallel paths" scope gap
blocked the Card's own named example ("2 of 3 app nodes lost, survivors at 150% of
declared capacity") from ever being built or tested. `core/journey_flow.go` gained
support for a `DeclaredJourney.Path` element naming more than one node ID, joined by
`"|"` — deliberately NOT a `workload.schema.json` change (`Path` stays `[]string`,
no version bump needed there; a `Path` with no `"|"` behaves identically to before).
`JourneyFlowResult` gains `ReachedByGroup` (which of each `Path` position's own
declared members were actually reached, position-for-position) and `JourneyHopFlow`
gains `GroupIndex` (the destination's own position) — together letting a caller see
which parallel members carried a journey and which didn't, under a fault. `ComponentLoad`
gains `LoadDivisionNote`, populated with the real even-split assumption text ONLY for
a component that is actually a member of a currently-reached parallel group of more
than one — PC-126's own "even-split rule tagged assumed and visible in output"
criterion, now genuinely exercised rather than recorded-but-dormant.

## canvas.schema.json 1.2.0 — 2026-09-26 (PC-136: CanvasNode.service_id added)

**Additive change to `canvas.schema.json` only** — the other six schemas are
untouched and remain at their current versions.

`CanvasNode` gains one new, optional field: `service_id` — the real provider
Terraform resource_type string (e.g. `aws_db_instance`, `aws_lambda_function`), the
same key `providers.Registry` is keyed by for the Terraform ingest path. Found during
PC-127 live verification: a canvas-authored node carried no way to identify which
specific AWS/Azure service it represents (`CanvasNode.Type` is a structural category —
`compute` cannot distinguish EC2 from Lambda, which carry different PC-107 capability
levels), so `core/trace.go`'s own capability gate correctly, but uselessly, marked
every canvas-built journey `not_assessable` at its first hop. `ingest/canvas.go`
resolves `service_id` through the exact same `providers.Registry.Lookup` the
Terraform path already uses — no second lookup implementation — stamping
`capability_level` onto `RawAttributes` identically to `ingest/build.go`'s own
`withCapabilityLevel`. Absent, unknown, or mismatched (a `service_id` whose registry
entry's own `NodeType` disagrees with this node's declared `Type`) all leave
`capability_level` unresolved — never guessed, never defaulted (I4) — surfaced by the
same existing `not_assessable` capability-check path a real out-of-vocabulary
Terraform resource already produces.

## report.schema.json 1.2.0 — 2026-09-26 (PC-122: Report.graph added)

**Additive change to `report.schema.json` only** — the other six schemas are
untouched and remain at their current versions.

`Report` gains one new, required field: `graph` (the PC-81 SVG diagram, or a real
failure message — never fabricated, same discipline `AssessResponse.Graph` already
established). Required because the HTML/PDF renderer (PC-122) embeds it directly,
per the Card's own explicit instruction ("Embeds the PC-81 SVG diagram directly, no
re-layout") — a report with no diagram string at all would leave that renderer with
nothing to embed. `core.BuildReport`'s own signature gained a `graphSVG string`
parameter (the caller renders it — core/ cannot do the subprocess I/O `render.SVG`
needs, I1), mirroring how `priceTable PriceTable` already threads pricing data the
pure engine cannot fetch itself into the same function.

## canvas.schema.json 1.1.0 — 2026-09-26 (PC-110: CanvasNode.sizing added)

**Additive change to `canvas.schema.json` only** — the other six schemas are
untouched and remain at their current versions.

`CanvasNode` gains one new, optional field: `sizing` (a free-form string map, same
shape and "untyped at this contract version" discipline as the existing `capability`
field). Keys are `core.Sizing`'s own canonical field names (`instance_type`, `count`,
`instance_class`, `allocated_storage_gb`, `storage_type`, `cache_node_type`,
`load_balancer_type`, `task_cpu`, `task_memory`) — never a Terraform attribute name.
`ingest/canvas.go`'s new `buildCanvasSizing` translates it into a typed `core.Sizing`
exactly the way `buildCanvasCapability` already translates `capability` into a typed
`core.CapabilityModel`. A key absent or blank leaves that `Sizing` field nil —
`cost_unknown` for that dimension, never a guessed default (PC-110's own explicit
acceptance criterion).

## report.schema.json 1.1.0 — 2026-09-26 (PC-132: ReportCostSection.usage_based_charges added)

**Additive change to `report.schema.json` only** — the other six schemas are
untouched and remain at their current versions.

`ReportCostSection` gains one new field: `usage_based_charges` (an array of
`UsageBasedCostEntry` — NAT gateway data processing, internet egress, cross-AZ
transfer, and LB data-processed LCU charges computed from declared traffic, PC-132).
Always present (never omitted/null, same "no null in JSON" discipline as
`Inventory`/`Delta`/`Assumptions`) — empty when no journeys are declared or no price
table was available, both distinguishable elsewhere in the same report (`Traffic.
Available`, `Cost.Available`), so no separate `unavailable_reason` is duplicated on
this field itself.

## workload.schema.json 1.3.0 — 2026-09-26 (PC-132: DeclaredJourney payload sizes added)

**Additive change to `workload.schema.json` only** — the other six schemas are
untouched and remain at their current versions.

`DeclaredJourney` gains two new, optional fields: `avg_request_bytes` and
`avg_response_bytes` (average payload size per request/response) — needed only for
byte-based usage cost (NAT gateway data processing, data transfer, LB data-processed
LCUs, PC-132). Never inferred, per the Card's own explicit instruction: a journey
missing either produces a real `not_assessable` usage-cost result naming the missing
field, not a guessed payload size.

## report.schema.json 1.0.0 — 2026-09-26 (PC-120: seventh frozen contract, new)

New contract, generated from `core.Report` — the report assembly endpoint's own root
type (`GET /sessions/{id}/versions/{n}/report`). No migration note applies to a new
contract's first version; recorded here so its own future changes have a starting
point to diff against.

**Why this contract exists at all, despite Design §5's own earlier v6 forward-note
guess** ("most likely no new contract for the report itself... a pure projection of
results the other five/six contracts already govern," `docs/preflight-technical-
design.md` §5): "projection" describes the report's own VERDICT logic — it invents no
new pass/fail/score, every value traces to an existing engine's own finding/control/
cost record by ID. It does NOT mean the report's own wire shape needs no schema: a
real, structured response spanning six other engines' output (findings, PC-121's three
compliance catalogs, PC-125/126's traffic/load, PC-117's cost, NFR conformance, an
assumptions appendix) is exactly the kind of "genuine new producer" `canvas.schema.
json`'s own precedent already established earns a contract. Design §5 and CLAUDE.md
§8's contract counts are updated in this same change (six frozen contracts → seven).

## workload.schema.json 1.2.0 — 2026-09-25 (PC-135: DeclaredJourney.iam_check added)

**Additive change to `workload.schema.json` only** — the other five schemas are
untouched and remain at their current versions.

`DeclaredJourney` gains one new, optional field: `iam_check` (a new `JourneyIAMCheck`
object — `principal_id`, `action`, and optional `resource_arn`). A caller holding a
pre-1.2.0 workload document is unaffected: the field is `omitempty`, so a document
without it parses exactly as before, and `core.ComputeJourneyFlow` treats a nil
`IAMCheck` exactly as it did before this field existed — network-only structural flow.

**Scope, stated explicitly:** the IAM check applies only to a journey's own FINAL hop
(its ultimate target), not per intermediate hop — see `DeclaredJourney.IAMCheck`'s own
doc comment (`core/workload.go`) for why: the Card's own named examples (Lambda → S3,
ECS task → Secrets Manager) are both end-to-end service calls, and a per-hop version
would need a principal/action/resource declared for every intermediate hop, real,
separate, larger scope this ticket does not attempt.

## workload.schema.json 1.1.0 — 2026-09-25 (PC-124: DeclaredJourney, service_time_ms added)

**Additive change to `workload.schema.json` only** — the other five schemas are
untouched and remain at their current versions.

`core.Workload` gains two new, optional fields: `journeys` (a new `DeclaredJourney`
array — id, name, ordered component `path` (IR Node.IDs, minimum 2), protocol, port,
criticality, and pointer `peak_rps`/`steady_rps`) and `service_time_ms` (a per-node-type
latency budget map, same shape and "absence means unknown" discipline as the existing
`capacity` map). A caller holding a pre-1.1.0 workload document is unaffected: both
fields are `omitempty`, so a document without them parses exactly as before.

**Why this was necessary, not scope creep:** journeys were already a real concept in
this codebase (PC-14/82's own `core.Journey` computes per-journey reachability), but
there was no first-class, architect-declared version of one with load attached — the
Card's own words, "make it explicit." Named `DeclaredJourney` here, deliberately
distinct from the pre-existing `core.Journey` (`core/simulate.go`) — that type is a
*computed* `/simulate` response fact ("did this entry-point-to-target path survive"),
a different concept that happens to share the English word; a real Go name collision,
resolved by not reusing the same identifier for two different things.

**No second capacity concept, per the Card's own explicit instruction:**
`DeclaredJourney` carries no capacity field of its own — per-component capacity is
still `Workload.Capacity` (unchanged, pre-existing), looked up via a new, GENERAL key
convention this ticket establishes and records (`core.JourneyCapacityKey`: a node's
own `NodeType` string, suffixed `_rps` — e.g. `"managed_database_rps"`). This is a new
convention, not a pre-existing GENERAL one being reused — `core/simulate.go`'s own
`SurvivingCapacity` call already reads this same map, but via one single hardcoded
key (`"app_node_rps"`, scoped only to its own PC-82 container_workload capacity
check), not a scheme any caller can resolve for any component's own node type. That
pre-existing key is left exactly as it was (renaming or removing it would break
`core/simulate_golden_test.go`, which depends on it being declared) — the new
convention is additive, living alongside it.

`core.EvaluateJourneyLoadReadiness` (`core/journey.go`) is the acceptance criterion
made real: a journey missing `peak_rps`, `steady_rps`, referencing an unknown IR
component, or missing a capacity declaration (by the new convention) for any real
component's node type is honestly `not_assessable`, naming the specific gap —
structural flow (PC-125) still works regardless; only *load* results are gated on
this. Full load distribution and bottleneck detection remain PC-125/126's own job;
this is the readiness gate those tickets build on.

`golden/workload.yaml` updated: `app_node_rps: 500` is kept, unchanged (load-bearing
for the pre-existing `/simulate` capacity check), with new keys
(`container_workload_rps`/`managed_database_rps`/`load_balancer_rps`/`dns_rps`) added
alongside it. A `checkout` journey (DNS → ALB → EKS → RDS, fully declared, matching
every new capacity key above) and a `settlement` journey (EKS → SQS) deliberately left
without `peak_rps`/`steady_rps` — a real, checked `not_assessable` case, the same
"golden fixtures include a checked not_assessable case" discipline PC-15's own
findings fixtures already established.

## ir.schema.json 1.3.0 — 2026-09-25 (PC-133: IAM policy model added)

**Additive change to `ir.schema.json` only** — the other five schemas are untouched
and remain at their current versions.

`core.Node` gains three new, optional fields: `iam_identity_policies` (a new
`PolicyDocument` array), `iam_trust_policy`, and `iam_resource_policy` (both single,
optional `PolicyDocument`s). `PolicyDocument` is a new type: `{id, version, statements,
provenance}`, where each `PolicyStatement` is `{sid, effect, principal, action,
not_action, resource, not_resource, condition}` — the real AWS IAM policy JSON shape,
field for field. `principal` and `condition` are untyped (`any`/free-form object):
AWS's own real JSON allows several genuinely different shapes for each (a bare
string, `"*"`, or an object with `AWS`/`Service`/`Federated` keys for `principal`; an
arbitrarily nested object for `condition`) — normalizing them into one fixed shape
would mean silently discarding whichever shape didn't fit, exactly what "wildcards
and conditions must be represented faithfully" (this ticket's own acceptance
criterion) forbids.

A caller holding a pre-1.3.0 IR document is unaffected: all three fields are
`omitempty`, so a document without them parses exactly as before.

**Why this was necessary, not scope creep:** identity nodes (`NodeTypeIdentity`) have
existed since PC-13/78, but only as a structural fact ("this role exists") — no
engine could answer "can this role read this bucket/table/secret" without the actual
policy documents in hand. PC-134 (policy evaluation) and PC-135 (integration into
traces/failure-lab/compliance) both need this data to exist in the IR first.

**Modelled scope**, stated directly (this ticket's own acceptance criterion): a
principal (role, already modelled), identity policies (inline `aws_iam_role_policy`
and managed `aws_iam_policy` + `aws_iam_role_policy_attachment`), a trust policy
(`aws_iam_role`'s own `assume_role_policy`), and a resource-based policy (e.g.
`aws_s3_bucket_policy`). **Not modelled**: policy variables (`${aws:username}`),
permission boundaries, service control policies (org-level, outside any single
Terraform bundle), IAM Access Analyzer findings, and condition *evaluation* beyond
verbatim storage (PC-134's own job — every condition key is preserved here regardless
of whether PC-134 ever learns to evaluate it, so that engine can report an
unsupported one as `not_assessable` rather than silently ignoring it).

Terraform ingest populates this from real resource attributes: `assume_role_policy`
(hand-verified against the golden AWS bundle's own real trust policies — 3 real
roles, `golden/aws/iam.tf`), and a new, dedicated synthetic fixture
(`ingest/testdata/iam-fixture/`, since golden/aws has none of the other three
resource types at all) for inline/managed identity policies and a resource-based
bucket policy — real JSON heredoc strings (never `jsonencode()`, a Terraform function
call this project's own static-literal ingest scope, CLAUDE.md §15, cannot capture at
all), exercising both the bare-string and array shapes of `Action`/`Resource` AWS
itself allows.

## ir.schema.json 1.2.0 — 2026-09-25 (PC-115: Node.Sizing added)

**Additive change to `ir.schema.json` only** — the other five schemas are untouched
and remain at their current versions (`canvas.schema.json` deliberately not bumped;
same reasoning as PC-111's own entry below — sizing authoring in the canvas UI is
separate, not-yet-built work).

`core.Node` gains one new, optional field: `sizing` (a new `Sizing` struct, entirely
`omitempty` at both the field and the struct-field level — mirroring
`CapabilityModel`'s own "every field a pointer, absence means not known, never a
default" discipline). A caller holding a pre-1.2.0 IR document is unaffected: every
node that existed before this ticket simply carries no `sizing`, exactly as if the
field had always been absent-and-optional.

**Why this was necessary, not scope creep:** the IR deliberately never carried sizing
before this — no failure scenario needed it (scenario-driven minimalism, CLAUDE.md §8).
The cost engine (PC-116/117) does: computing a real per-component cost estimate needs
to know what an architect actually declared (instance type/class, count, storage),
never a "typical" assumed size. `Node.RawAttributes` could not honestly serve this
role — it is a provider-specific, verbatim, untyped bag; `Sizing` is canonical,
cross-provider vocabulary the same way `CapabilityModel` already is.

**Region is deliberately NOT part of `Sizing`**: `Workload.Regions` (unversioned,
`workload.schema.json`, unchanged) already declares it — no per-node region field
exists anywhere in the IR (`core/simulate.go`'s own `region_loss` doc comment already
states this), so adding one to `Sizing` would duplicate, not reuse, the real source of
truth.

**Count is a sizing fact, not a capacity fact** — `Workload.Capacity` (PRD §4's own
capacity semantics) remains the only source of truth for capacity; `core/sizing_test.go`
structurally proves nothing in `core/simulate.go`'s capacity-handling code reads
`Sizing` at all.

Terraform ingest populates `Sizing` from real resource attributes on the golden
AWS bundle: `instance_class`/`allocated_storage`/`storage_type` (RDS),
`node_type`/`num_cache_clusters` (ElastiCache), `load_balancer_type` (ALB), and
`instance_types[0]`/`scaling_config.desired_size` merged from a companion
`aws_eks_node_group` resource onto its owning EKS cluster node (`ingest/sizing.go`).
Golden fixtures regenerated; diff is `sizing` additions plus `version_hash` and
`schema_version` only, hand-verified.

## ir.schema.json 1.1.0 — 2026-09-25 (PC-111: routes, IGW, NAT — Edge.RawAttributes added)

**Additive change to `ir.schema.json` only** — the other five schemas are untouched
and remain at their current versions (`canvas.schema.json` deliberately not bumped;
see below).

`core.Edge` gains one new, optional field: `raw_attributes` (an untyped object,
`omitempty` — mirroring `core.Node.RawAttributes`, already frozen since v1). A caller
holding a pre-1.1.0 IR document is unaffected: every edge that existed before this
ticket (`contained_in`, `depends_on`, etc.) simply carries no `raw_attributes`, exactly
as if the field had always been absent-and-optional.

**Why this was necessary, not cosmetic:** a `routes_to` edge (route table -> IGW/NAT)
needs to carry its destination CIDR — `core/internal/analyse`'s longest-prefix-match
route selection (AWS VPC User Guide, "How route priority works," verified 2026-09-25)
needs the real CIDR, not just "a route exists to this target." A plain `(From, To,
Type)` triple cannot express that; `Node.RawAttributes` already established the exact
precedent (real per-element data a fixed field set can't anticipate) this reuses on
the edge side.

**`canvas.schema.json` NOT bumped, a stated decision, not an oversight:** PC-111's own
Card asks for a canvas schema bump "if the canvas can author routes." It cannot yet —
the Architect Workspace's route/IGW/NAT authoring UI is separate, not-yet-built work
under the PC-96 epic. When it lands, `canvas.schema.json` gets its own version bump and
migration note at that point, on the same "extended when a genuine new producer earns
one" standard `canvas.schema.json` was itself added under.

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
