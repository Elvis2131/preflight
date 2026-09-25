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
| Go module + 3-process skeleton | scaffolded: `cmd/assessd` (P1), `cmd/reasond` (P2), `cmd/runnerd` (P3) all build and run (PC-10). Credential boundary is now structural, not just aspirational: `cmd/runnerd/internal/creds` (empty — no real credential-loading code exists yet, PC-25) is compiler-enforced importable only from `cmd/runnerd/`, proven the same synthesize-and-build way `core/boundary_test.go` proves I1; a second live check (`go list -deps`) confirms `cmd/assessd`/`cmd/reasond` currently import zero real cloud SDKs and no dependency on `validate/` itself — closing PC-10's last open criterion, previously an honestly-recorded ADR-003 gap ("needs a check, not just discipline") |
| Invariants I2, I4 | type-level enforcement + tests in `core/` (`Provenance`/`Tagged[T]`, `Assessment[T]`) (PC-6) |
| Invariant I1 (structural half) | `core/internal/{ir,analyse,simulate}` compiler-enforced private; proven by `core/boundary_test.go`, not just asserted |
| IR design (`core/ir.go`, `docs/IR_DESIGN_NOTE.md`) | canonical/capability boundary resolved for all 8 golden node types + WAF's edge case; resolution-state propagation structurally enforced via `core/internal/analyse.AssessNode`/`AssessEdge` (PC-11) |
| Invariant I1 (defense-in-depth half) | `.golangci.yml` (depguard + forbidigo), migrated to the v2 schema and actually run for the first time (PC-6) — running it surfaced and fixed 3 real config bugs (a glob that never matched direct children of `core/`, a misunderstood `pkg` field), each caught by negative control; `.github/workflows/ci.yml` runs it in CI |
| Contract schemas (`contracts/`) | all 6 generated from Go structs + validated against real samples (PC-7 froze the first 5; `canvas.schema.json` 1.0.0 added PC-86, decided explicitly before canvas ingestion was built rather than left implicit); `finding.schema.json` at 1.2.0 (1.1.0 `FailureMode` dimensions, PC-17; 1.2.0 `EvidenceRef.Attribute`, PC-18), others unchanged at 1.0.0 — schemas version independently per Design §5, see `contracts/CHANGELOG.md` |
| `ingest/` HCL parser | real HCL v2 parsing, MVG check, golden/aws round-trips losslessly (64 resources: 11 nodes + 1 mapped edge + 52 out-of-vocabulary) (PC-12) — WAF↔ALB association edge captured via `providers/aws`'s edge-mapping mechanism; clean/broken bundles verified to differ on it |
| Routes, IGW, NAT gateway (`ingest/routes.go`, `core/routing.go`, `core/internal/analyse/routing.go`) | PC-111: `ir.schema.json` bumped to 1.1.0 (`Edge.RawAttributes`, additive — a route's real destination CIDR, which a plain `(From, To, Type)` triple can't express). Both real Terraform route shapes ingest correctly (`aws_route_table`'s own inline `route{}` blocks — repeated ones, via a new `ParsedResource.NestedBlocks`, not just the last one — and the standalone `aws_route` resource); an unsupported target (VPC peering/TGW/endpoint/...) produces a real `not_assessable` finding naming the target, never a fabricated edge. Route selection is real longest-prefix-match (`core.LongestPrefixMatch`) and "public subnet" is derived from actual routes, not trusted from a label (`core.IsPublicSubnet` + a real finding when a `tags.Tier=public` subnet's route table has no IGW route) — both hand-verified against AWS's own documented worked examples (`docs.aws.amazon.com/vpc/latest/userguide/route-tables-priority.html` and `.../subnet-route-tables.html`, cited in the code and in `tests/aws-conformance/routing`'s own conformance tests), all negative-controlled |
| Security Group evaluation engine (`core/internal/analyse/securitygroup2.go`, `core/securitygroups.go`) | PC-112, supersedes PC-79 (reopened, then correctly closed here): real, stateful SG evaluation — allow-only/implicit-deny, CIDR **and** SG-reference sources (membership-based, verified against `docs.aws.amazon.com/vpc/latest/userguide/security-group-rules.html`'s own "Security group referencing" section and its ALB→web→DB worked example), multi-SG union at the ENI, protocol/port matching, and real statefulness (`EvaluateConnection` never re-checks the return leg — verified against `docs.aws.amazon.com/AWSEC2/latest/UserGuide/security-group-connection-tracking.html`'s own exact wording, negative-controlled by proving a version that DID re-check the return leg fails the statefulness test). Ingest reuses PC-111's own `NestedBlocks` mechanism to correctly capture every `ingress{}`/`egress{}` block (not just the last one) plus every standalone `aws_security_group_rule`, merged onto the owning SG node's `RawAttributes` — no IR schema change needed (`RawAttributes` is already untyped). PC-79's own `sgPermits` now delegates to this real engine rather than reimplementing matching — proven by a real cross-check test (`TestExactlyOneSGEvaluationPath`) that both paths agree, and all of PC-79's own tests (including its negative control) pass unchanged. A CIDR match that partially (not fully) overlaps a rule's range returns a stated not_assessable-style reason rather than guessing |
| Network ACL evaluation engine (`core/internal/analyse/nacl.go`, `core/nacl.go`) | PC-113: real, deliberately-separate-from-SG NACL evaluation (never collapsed — the reference project's own guidance the Card cites, and AWS's own docs, both treat them as different abstractions) — numbered rules evaluated ascending with first-match-wins (`docs.aws.amazon.com/vpc/latest/userguide/vpc-network-acls.html`), the real "\*" catch-all deny (`.../default-network-acl.html`), both allow **and** deny rules (unlike SGs), and real statelessness — `EvaluateNACLConnection` separately checks BOTH the forward leg and the return leg on the ephemeral port range (the opposite of SG statefulness), negative-controlled by proving a version that skipped the return-leg check passes when it shouldn't. Ephemeral range defaults to `1024-65535` — AWS's own documented worked-example default (`nacl-examples.md`), never a hardcoded OS-specific range, stamped on the decision so a caller can tag it assumed. `core.EvaluateNACLPath` proves the same-subnet criterion directly: traffic between two resources in one subnet never evaluates the NACL at all, even a deny-all one (`docs.aws.amazon.com/vpc/latest/userguide/vpc-network-acls.html`: "not as it is routed within a subnet"), negative-controlled. Ingest mirrors PC-112's own `aws_security_group`/`aws_security_group_rule` mechanism exactly for `aws_network_acl`'s inline `ingress{}`/`egress{}` blocks plus the standalone `aws_network_acl_rule` resource — golden bundles have zero NACL resources (confirmed, PC-78/79's own scope decision), so this is entirely synthetic-fixture coverage, same discipline PC-14 established before ever touching real data |
| Per-request simulation trace (`core/trace.go`, `server/trace.go`, `POST /sessions/{id}/trace`, `trace` MCP tool) | PC-114: combines PC-111/112/113 into one ordered, provenance-tagged, explainable pipeline — resolve destination, resolve source (skipped for an internet-originated request), a capability gate, route selection, Network ACLs at each subnet boundary (skipped for same-subnet traffic, `core.EvaluateNACLPath`'s own short-circuit; an internet-originated request still crosses the destination subnet's own NACL, represented with an all-permissive placeholder standing in for "the internet has no NACL of its own"), Security Groups at the destination ENI (SG statefulness stays automatic — no separate response-path step, per PC-112's own engine), and a structural (not live) target-health check. Neither side's own IP is modelled in the IR at all; each side's subnet `cidr_block` (captured verbatim onto `RawAttributes` since PC-78) stands in for it, falling back to an explicit "unknown address" placeholder only when no subnet is resolvable — a CIDR-restricted rule against that placeholder reports not_assessable rather than guessing allow or deny. **Scope decision**: the Card's own acceptance criterion gates the pipeline on a "REQUEST_SIMULATION capability level" that PC-107 (service capability registry, not yet built) would define; rather than block on it, `requestSimulationCapable` is a narrow, stated placeholder answering only the question this pipeline needs, documented as the seam PC-107 will replace. Both the same-subnet NACL skip and the capability gate are negative-controlled |
| CIDR allocator (`core/internal/analyse/cidr.go`, `core/cidr.go`, `providers/aws/cidr_rules.go`) | PC-106: ported from the reference project's `cidrAllocator.ts` as a first draft (ADR-004), then re-verified against AWS's own VPC docs rather than trusted — VPC/subnet size range `/16`-`/28` and the 5 reserved addresses per subnet (network, VPC router, DNS at base+2, future use, broadcast) both cited (`docs.aws.amazon.com/vpc/latest/userguide/vpc-cidr-blocks.html`, `.../subnet-sizing.html`). Rule constants live in `providers/aws/cidr_rules.go` as plain Go consts (a deliberate, stated divergence from the per-resource-type YAML pattern — these are provider-wide numeric facts, not a `ResourceMapping`), never hardcoded in `core`. `AllocateSubnets` is deterministic (largest-first stable sort, a plain slice-scan first-fit — never map iteration) and negative-controlled: disabling its own overlap check is confirmed to break both the direct test and a 200-trial seeded property test (fixed RNG seed, so it's reproducible, not a fuzzer) before being reverted. `ValidateSubnetOverride` rejects an architect's manual override with a distinct code per rule (undersized/oversized/overlapping/out-of-VPC), each with its own conformance test |
| Service capability registry (`core/capability.go`, `providers/mapping.go`) | PC-107: the named 9-rung ladder (METADATA_ONLY → ... → FULL_BEHAVIOR), stored per-mapping as `capability_level` and validated at `Load()` time against `core.MaxImplementedCapabilityLevel(NodeType)` — an honest, structurally-enforced ceiling (the highest rung any REAL engine in this codebase reaches for that NodeType today, e.g. `network_boundary` → NETWORK_BEHAVIOR from PC-111/112/113, `managed_database`/`cache` → FAILURE_SIMULATION from their own cited `FailoverMapping`, everything else capped lower) — a mapping claiming more than its NodeType's engines actually implement is refused at load, negative-controlled. Every existing AWS/Azure mapping now carries one; PC-114's own `requestSimulationCapable` placeholder is replaced by reading this real, per-service value (`RawAttributes["capability_level"]`, stamped by `ingest/build.go`) instead of a static NodeType whitelist — the exact seam PC-114 itself named. Palette modestly expanded (S3, Lambda, DynamoDB, SNS), each capped at CONFIGURATION since no dedicated engine models their specifics yet — raising any of them requires that engine, not just the palette entry. **Scope note**: the UI (palette/inspector coverage display) is out of this session's scope — see the final blocker summary |
| Sizing fields (`core.Sizing`, `ingest/sizing.go`) | PC-115: `ir.schema.json` bumped to 1.2.0 (`Node.Sizing`, additive; `canvas.schema.json` deliberately not bumped — no canvas sizing-authoring UI exists yet). Every field is a pointer, same "absence is never a default" discipline as `CapabilityModel` — the cost engine's own `cost_unknown` (PC-116/117) falls straight out of a nil field, not a special case. Terraform ingest populates it from real golden-bundle attributes: RDS's `instance_class`/`allocated_storage`/`storage_type`, ElastiCache's `node_type`/`num_cache_clusters`, ALB's `load_balancer_type`, and EKS's `instance_types[0]`/`scaling_config.desired_size` merged from the companion `aws_eks_node_group` resource onto the owning cluster node (sizing data for EKS genuinely lives on a separate Terraform resource from the mapped node). Region is deliberately not part of `Sizing` — `Workload.Regions` already owns it; the Card's own "already present on containers" wording is corrected in code, since no per-node region field exists anywhere in the IR. Count is a sizing fact only: a structural test parses `core/simulate.go`/`workload.go` and proves neither references `Sizing` at all, negative-controlled. Golden fixtures regenerated; diff is `sizing` additions plus version metadata only, hand-verified |
| Pricing snapshot service (`docs/adr/ADR-006-*.md`, `pricing/`, `cmd/runnerd/internal/pricingfetch/`, `server/pricing.go`) | PC-116: ADR-006 records real, live-verified findings (2026-09-25) rather than assumed ones — the Bulk Price List API needs **no AWS credentials at all** (a correction of the Card's own assumption), retains AWS's own dated version history, and its region files vary wildly in size (`AWSELB` 19KB, `AmazonS3` 473KB, `AmazonRDS` 27MB, `AmazonEC2` **481MB** — all sizes measured live, not estimated); EC2/NAT Gateway are deliberately deferred past v1 on that measured size, a stated gap. The real fetcher (`cmd/runnerd/internal/pricingfetch`) is compiler-private to `cmd/runnerd` (Go's own `internal/` rule — proven by actually attempting the forbidden import and watching the build fail, same technique `cmd/runnerd/internal/creds` already established for PC-10, whose own boundary test is extended here) even though it needs no credentials, since the boundary that matters is "no external network call reachable from assessd" (NFR-1/I1), not merely "no credential." `pricing/` (data types + its own SQLite store, ADR-002) has zero network capability of its own (structurally proven, `pricing/boundary_test.go`), so `server` safely imports it for three read-only endpoints (`GET /pricing/snapshots`, `GET /pricing/snapshots/{id}`, `POST /pricing/snapshots/{id}/activate`) without adding any network capability to P1. The fetcher was run for real against the live endpoint while building this (7090 real, normalized price entries stored from AWSELB/S3/ElastiCache/RDS, one real $0 Outposts entry surfaced and correctly handled, not treated as a bug) — checked-in fixtures for the parser's own unit tests are real, live-fetched AWS data (`cmd/runnerd/internal/pricingfetch/testdata/`), with a separate, deliberately build-tag-gated live-network test (`-tags live_pricing`) for manual reproof, the same "external dependency verified deliberately, not per-commit" discipline `cmd/check-conformance-links` already established |
| Cost computation (`core/cost.go`) | PC-117: pure `ComputeCost(ir, PriceTable, prov) CostReport` — `PriceTable`/`PriceRow` are core's own plain mirror of `pricing.Snapshot`/`PriceEntry` (core cannot import `pricing/`, same I1 direction `core/routing.go`'s `UnsupportedRouteInfo` already established for `ingest`), so the caller converts. Matches RDS/ElastiCache/ALB sizing (PC-115) against real price rows by the actual AWS attribute names (`instanceType`, `databaseEngine`, `deploymentOption`, `cacheEngine`, `usagetype`) — hand-verified against three individually live-fetched real price rows (RDS `db.r6g.xlarge`/PostgreSQL/Multi-AZ $0.899/hr, ElastiCache `cache.r6g.large`/Redis $0.206/hr, base ALB $0.0225/hr, 2026-09-25) against the real golden AWS bundle's own real sizing. `HoursPerMonthAssumption` (730) lives in exactly one place, stated as an assumption. A component with no sizing, an unrecognized engine, or no matching row is honestly `cost_unknown` with a real reason — never a guessed default, never silently folded into the total as zero (negative-controlled: an injected fake nonzero amount on an unpriced component is confirmed caught). Deterministic (same IR + same table → byte-identical `CostReport`, tested). Server-side wiring is complete: `server.Store.AttachPricingStore` optionally attaches a `pricing.Store` (a `nil` one preserves every pre-PC-117 caller's exact behavior — `Cost` stays `nil`, backward compatible, all existing tests pass unchanged); `AssessRequest.PriceSnapshotID` pins a specific snapshot, defaulting to the pricing store's own active one; an unresolvable pin is a real `snapshot_not_found` error, never silently ignored; `AssessResponse.Cost`/`StoredVersion.Cost` (new `cost_json` column, same `ALTER TABLE`-if-missing pattern as `workload_json`) round-trip identically through the GET read-back — all real, end-to-end tested against the golden AWS bundle |
| Cost as a scorecard dimension (`core/cost_delta.go`, `core/cost_findings.go`) | PC-118: `Scorecard.Cost *CostReport` is its own dimension — a pointer field, so `TestScorecardHasNoCompositeScoreField`'s reflection walk (kind `Ptr`, not numeric) doesn't and shouldn't flag it; the forbidden thing is one number folding every dimension together, not "no numbers anywhere." `ComputeCostDelta` is a dedicated type (`CostDeltaEntry`/`CostDeltaKind`), not folded into the existing compliance-shaped `DeltaEntry` — hand-verified on golden aws-broken → aws (RDS Single-AZ → Multi-AZ: +$327.77/mo; ElastiCache 1 → 3 nodes: +$300.76/mo; ALB unchanged, identical Terraform in both bundles), all real, live-fetched AWS prices. **Recorded decision** (the Card's own instruction: "don't let it be implicit"): when two versions were priced against different snapshots, per-component figures are never classified increased/decreased (that asserts a design change no honest comparison can support when AWS's own list price may have moved too) — only structural facts (added/removed/became_priced/became_unknown) are still reported; negative-controlled. A declared `monthly_cost_budget_usd` requirement (PRD §4's existing, general Requirement mechanism — no new Workload field) reused as-is: `hard` + exceeded → a real failing `finding.cost.budget-compliance` (using the existing `satisfied`/`unsatisfied` ComplianceStatus vocabulary, so the pre-existing compliance delta engine classifies it for free); `preference` + exceeded → `CostReport.BudgetExceeded` records the trade-off, never a Finding — both tested, both negative-controlled. `AssessResponse.CostDelta`/`CostSnapshotChanged` wired end-to-end through both `/assess` and the GET read-back |
| Declared journeys and traffic inputs (`core.DeclaredJourney`, `core/journey.go`) | PC-124: `workload.schema.json` bumped to 1.1.0 (`journeys[]` + `service_time_ms`, additive). Named `DeclaredJourney`, deliberately distinct from the pre-existing, COMPUTED `core.Journey` (`core/simulate.go`, PC-82's own `/simulate` response fact) — same English word, different concept, a real Go name collision resolved by not reusing the identifier. No second capacity field: per-component capacity stays `Workload.Capacity`, resolved via a new general convention this ticket records (`core.JourneyCapacityKey`: `NodeType` string + `_rps`) living alongside `core/simulate.go`'s own pre-existing single hardcoded key (`"app_node_rps"`, left untouched — renaming it would break `core/simulate_golden_test.go`) — a real, corrected finding: an earlier pass wrongly claimed `Workload.Capacity` had no reader before this ticket; grep found the existing one and this file's comments were fixed to match. `core.EvaluateJourneyLoadReadiness` gates load computation (PC-125/126's own job) on every input actually being declared — missing `peak_rps`/`steady_rps`, an unknown path component, or a missing capacity entry is honestly `not_assessable`, naming the gap; structurally verified DeclaredJourney carries no field with "capacity" in its name, negative-controlled. `golden/workload.yaml` gained a real `checkout` journey (fully declared, DNS→ALB→EKS→RDS) and a real `settlement` journey deliberately left without declared rps — a checked not_assessable case, hand-verified against the real golden bundle. **Scope note**: the NFR form's journeys editor and canvas path-picking (PC-87/canvas UI) are out of this session's scope — flagged in the final blocker summary, not silently dropped |
| Structural traffic flow (`core/journey_flow.go`) | PC-125: `ComputeJourneyFlow` walks a `DeclaredJourney`'s Path hop by hop, reusing PC-114's `BuildTrace` verbatim for each hop's own SG/NACL/route decision (zero duplication) — a blocked hop is reported not flowing, citing `BuildTrace`'s own concise reason. Fault-awareness reuses PC-14's own `ContainmentBlastRadius`/`SimulateLoss` (the *caller* computes the killed set; this engine only consumes it) — a hop touching a killed component stops there without even consulting `BuildTrace`, real end-to-end tested by actually killing a subnet via `ContainmentBlastRadius` and confirming the journey stops at the resource really contained in it. `JourneyInternetSentinel` (`"internet"`) mirrors `BuildTrace`'s own convention for an internet-originated first hop — required a real correction to PC-124's own golden `checkout` journey (it had started at the DNS record node, which has no subnet placement and can't act as a `BuildTrace` source at all; fixed to start with the literal sentinel, matching the Card's own example). Golden-bundle hand-verification is honest about a real, pre-existing gap: `golden/aws` has zero NACL resources (PC-113), so the real checkout journey is correctly reported as blocked at NACL resolution, not a fabricated success. **Scope limitation, stated**: "parallel paths" for load-balanced/multi-AZ components is not implemented — this IR's own node granularity has no per-AZ-instance node to enumerate paths between (a multi-AZ EKS cluster is one IR node regardless of how many AZs back it); building that needs a real ingest modeling change, separate, larger work. Negative-controlled (both the killed-set short-circuit and the real containment-blast-radius integration) |
| Golden fixtures (`golden/fixtures/`) | IR **and** findings fixtures, derived + hand-verified for both bundles (PC-15, complete) — 5 representative findings per bundle from the real PC-14/17/18/28 engines, including a checked not_assessable case |
| AWS conformance test tier (`tests/aws-conformance/`) | PC-119, head of the critical path for the Network & Request Engine work (PC-111/112/113/114): a citation-required test format (`harness.Verify`, `tests/aws-conformance/harness/`) distinguishing "matches our own contracts" (existing regression tests) from "matches documented AWS behaviour" (this tier) — a test missing its citation fails `go test ./...` itself, no separate lint tool. Backfills the RDS Multi-AZ / ElastiCache automatic-failover claims (`providers/aws/rds.yaml`/`elasticache.yaml`'s own citations) as real end-to-end tests (real `ingest.Ingest`, not read back out of the YAML that made the claim) — negative-controlled by actually breaking a mapped rule and confirming the failure names it and its source. `cmd/gen-conformance-report` statically parses every `harness.Spec{...}` via `go/ast` into `tests/aws-conformance/COVERAGE.md` (area test packages are separate `go test` processes with no shared memory, so static parsing is the only way to aggregate them). `cmd/check-conformance-links` is a deliberately periodic (weekly, `.github/workflows/conformance-links.yml`), not per-commit, citation-URL health check — an AWS doc page moving must never fail an unrelated commit. `networking/routing/cidr/placement/pricing` subpackages exist as stubs so PC-106/111/112/113 and PC-100 write directly against this format from day one |
| Version diffing / Assurance Delta (`core.ComputeDelta`, `core.Scorecard`) | 6-value `DeltaKind` enum (PC-83 added `resolved_risk`, PRD §5.7's own category, distinct from `improvement`: an unknown resolving into known-good vs. a known state getting better), every entry carries `Provenance`, no composite score anywhere (checked via reflection) — all 6 kinds hand-verified synthetically, `improvement`/`resolved_risk` also proven against the real golden bundles (PC-19, PC-83) |
| ADR & waiver objects (`core.ADR`, `core.Waiver`) | PC-20: schema captures who/what finding/when/why/against-which-version/optional-expiry (`Waiver`) and links to failure modes/controls by ID, not embedded copies (`ADR`) — both checked structurally by reflection. `core.ApplyWaivers` makes a waived finding display as `"accepted"` in the scorecard rather than being hidden (`Waiver.Applies`'s `versionNumber >= AgainstVersion` floor, not an exact-match gate — negative-controlled: an exact-match version check was proven to fail the very re-appears-every-iteration case this object exists to prevent). Enforcement (auto-expiry when the finding materially changed, not just when `ExpiresAt` passes) is P2, not built here; no server-side persistence/API for creating waivers exists yet either — out of this ticket's literal scope |
| Layer 1 structural analysis (`core/internal/analyse`) | min-cut, SPOF detection, capacity discipline, zone-loss/containment reachability, RTO/RPO feasibility — all hand-verified in isolation before touching real data (PC-14). 2 real bugs found and fixed this way |
| Network Engine: SG rule evaluation (`core/internal/analyse.FilterEdgesBySGRules`) | Built under PC-79 (commit 86c054e) as a standalone edge-prefilter: drops an edge when its destination's security groups don't actually permit inbound traffic from the source's own SGs, then hands the narrowed edge set to PC-14's `DetectSPOFs` completely unchanged — hand-verified on a synthetic fixture (two topologically-redundant routes, one blocked by a missing SG rule) that a real SPOF only becomes visible after SG filtering. **Correction**: PC-79 was closed in error against a stale ticket description — a prior comment on PC-79 had already resolved it as absorbed into PC-112 (the Security Group evaluation engine, stateful/allow-only/CIDR+SG-reference/explainable, under the Network & Request Engine epic PC-97) and said to leave it open until PC-112 closes; that comment was missed. PC-79 has been reopened. The code/tests above stay as a permanent regression check, but `sgPermits`'s ingress-only, no-CIDR, non-stateful logic is superseded scope, not the final SG semantics — PC-112 owns the one real SG evaluator, and `FilterEdgesBySGRules` will be rewired to call it (CIDR support arrives once PC-106/PC-111 give subnets real ranges). See CLAUDE.md §19's new first rule (read every ticket comment, not just the description) for the process fix that followed |
| `providers/aws/` | 8 golden resource types + VPC/subnet/NAT/security-group substrate (PC-78) mapped (YAML data + loader), every field's provenance explicit, 4 assumed defaults doc-verified (PC-13); zone-kill now reports real, hand-verified losses on the golden bundle (PC-78); own `FailoverMapping` data now carries doc-verified replication/failover wording per resource (PC-22 groundwork — relocated out of `core/internal/analyse`, which must stay provider-agnostic, before it could have handed Azure SQL AWS's own wording verbatim) |
| `providers/azure/` + `golden/azure/` + `golden/azure-broken/` | 8 golden resource types mapped and real, `terraform validate`-passing golden bundles built on them (PC-22, PC-29) — Azure DNS, App Gateway + its WAF policy, AKS, Azure SQL, Cache for Redis, Service Bus, RBAC role assignment; Front Door explicitly out of scope; shares `providers/`'s schema with AWS. `core.BuildFindings` and `server.Assess` both used to be hardcoded to AWS (node IDs/attribute keys; the AWS-only provider registry) — **fixed under PC-29**: genericized to run against any `managed_database` node (AWS's own finding IDs regression-tested unchanged) and merged both providers' registries (`providers.Merge`). A real live-agent iteration cycle converges on Azure in 1 round with correct `resolved_risk`/`improvement` classification (`demo/pc29-azure-live-agent-run/`). Azure still has no subnet-level AZ placement to run zone-kill against at all — a real model difference with its own follow-up ticket, not silently produced as working (see `golden/azure/README.md`) |
| `server/` (SQLite store, `/assess`, `/simulate`, `GET /sessions/{id}/versions/{n}`, MCP tools) | real, running server (PC-21): SQLite-backed session/version store (ADR-002, also closes PC-9's remaining criterion), `/assess` measured under 5s, HTTP and MCP proven to call the identical function, `graph` field is real, deterministic server-rendered SVG (PC-81 — `core.RenderDOT` + Graphviz via `render/`; byte-identical output for a GIVEN fixed Graphviz install is now checked directly against a committed fixture (`render/testdata/golden_aws.svg`), so a version drift between CI runs fails loudly instead of passing silently — CI installs Graphviz and prints `dot -V` too, a real bug this project's own audit caught: the workflow never installed it at all until asked directly whether the version was pinned); `/simulate` (PC-82) declares one fault against an already-assessed version, reusing PC-14's fault-injection engines directly — `region_loss` and `node_loss` (PC-88) both hand-verified live against golden/aws. `GET /sessions/{id}/versions/{n}` (PC-94) re-serves a stored assessment with zero re-ingest/re-analyse — the API-side half of canvas's own "no persistence" gap, surfaced by a proactive audit rather than a fourth reactive discovery. Failures are structured now (PC-95): a stable `{error_code, message}` body plus a real status per failure mode (`session_not_found`/`version_not_found` → 404, `invalid_request_body` → 400, `invalid_workload`/`invalid_bundle`/`insufficient_model` → 422) instead of one generic 400 for everything — additive, existing text-matching callers still work unchanged. All three MCP tools (`assess`, `simulate`, `assess_canvas`) now register and run with real, correctly-generated input/output schemas (PC-93) — `assess_canvas` didn't exist as an MCP tool at all until this fix; a real MCP client/server round trip (in-memory transport, real wire marshaling and schema validation) also caught a genuine, separate bug in `core.BuildFindings`' NAT-gateway-redundancy check, since fixed |
| API docs (`/openapi.json`, `/swagger`) | OpenAPI 3.1 spec reflected directly from `AssessRequest`/`AssessResponse` via `invopop/jsonschema` (not hand-written — same discipline as `cmd/gen-contracts`), rendered with Swagger UI 5.29.1 |
| Agent-iteration acceptance test (`server/agent_iteration_acceptance_test.go`) | CI-deterministic: drives a mutable working copy of `golden/aws-broken` through the real author→evaluate→modify→re-evaluate loop via `server.Assess`, applying scripted fixes per unsatisfied finding, and proves convergence in 2 of a written-down 5-iteration bound (PC-28; bound decided up front in `docs/PC-28-ITERATION-BOUND.md`, not discovered empirically). A separate live-agent demo run is PC-29's job, not built here |
| `exporters/` | FIS + Litmus chaos-experiment exporters (PC-23) — real, doc-verified schemas (not guessed), each export carries its own Provenance distinct from the source finding; a region-wide FIS variant (PC-29) verifies `/simulate`'s `region_loss` prediction. FINOS CALM exporter (PC-27, `exporters/calm.go`): validates against the real, vendored official CALM schema (`exporters/calm_schema/`, fetched from `finos/architecture-as-code`, not guessed) both at export time and in CI (`go test ./...` already runs it); a deliberately malformed document is proven to actually get rejected, not just the happy path proven to pass. The export states its own one-way/lossy nature (Provenance/ResolutionState/CapabilityModel not round-trippable) directly inside the exported JSON's own `metadata.notice` field, not only in a design doc. `contained_in` edges map to CALM's `deployed-in` relationship, every other edge type to `connects` — no changes to `core/` were needed |
| `canvas/` | Drag-and-drop architecture authoring canvas shell (PC-85, React + `@xyflow/react`) — golden-vocabulary-only palette (11 real `core.NodeType`/6 `core.EdgeType` values, mirrored from `core/ir.go`, not redefined), clean `CanvasDocument` serialization (no UI-only fields — unit-tested against React Flow's own real internal fields), verified live in a real browser (Playwright), not just typechecked. Now wired to the backend (PC-86): `POST /sessions/{id}/canvas` — `ingest.IngestCanvas`, a second IR producer symmetric to HCL ingestion, sharing the identical downstream pipeline (`server.assessFromResult`) so the two paths can't quietly diverge; a dangling edge produces real `not_assessable` findings, never a crash or a guess. `CanvasDocument` is now `canvas.schema.json`, the sixth frozen contract, decided explicitly before this ingestion logic was written — no capability form yet (PC-87) |
| `viewer/` | PRD §6's thin, read-only secondary web UI (PC-90) — reads the API only, authors nothing. PC-92: a per-finding scorecard timeline table (V1..Vn columns), colored by the server's own real `DeltaKind` classification, no composite score/trend line anywhere (reviewed explicitly against CLAUDE.md §10 before writing the one function that builds it). Required a new endpoint, `GET /sessions/{id}/versions` (`server/history.go`) — a real gap found starting this ticket: nothing could answer "how many versions does this session have." PC-91: renders PC-81's real SVG verbatim (no client-side transformation) plus that version's own real `assurance_delta` list, no new diff computation. Verified live in a real browser against a real 2-version golden broken→clean transition for both, not just typechecked |
| `reason/`, `ui/` | package stubs only (`doc.go`), no logic |
| `validate/` Rung 1 (`validate/rung1.go`, `validate/rung1/`) | PC-24: a real docker-compose stack (Postgres + Toxiproxy, zero cloud credentials) with a Toxiproxy-injected "cut" fault on the app-tier-to-database edge — narrowed to that one edge/fault shape deliberately (matches the golden "database failure" scenario; AZ/region loss are placement facts, not one edge, and identity failure isn't a network dependency at all). Run live end-to-end (real Docker containers brought up and torn down by the test itself, not mocked): a Read returning `io.EOF` within ~1ms is the real, hand-verified signal Toxiproxy's disable produces — bare dial success/failure turned out NOT to distinguish the fault (Docker's own port-publishing layer still completes the TCP handshake either way). Tagged `observed`/`functional`/Rung 1; `core.Provenance` gained a missing structural check this ticket needed (`validateProvenanceEvidenceTierRung`) so a Rung 1/2 result can never carry `EvidenceTierPerformance`/`EvidenceTierResilience` — proven against this specific harness's own output, not assumed. Rung 2/3 (LocalStack, real ephemeral cloud) remain stubs (PC-25, P3 credentialed) |

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
- `aws_subnet.data_a` → **PC-80 closed this gap**: `aws_db_subnet_group`/
  `aws_elasticache_subnet_group` are now mapped (`network_boundary`,
  `reference_edge_type: contained_in` — same pattern `aws_subnet` itself uses),
  giving RDS/ElastiCache real, two-hop containment visibility with zero changes to
  `ingest`/`core/internal/analyse` — both were already generic enough. Exactly
  `[aws_db_instance.payments, aws_db_subnet_group.payments,
  aws_elasticache_replication_group.payments, aws_elasticache_subnet_group.payments]`
  in both bundles (identical — neither bundle's own defects touch subnet placement).

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
*.schema.json` correctly has `minLength` applied). Worked around at the time by using
`any` for the MCP tool's output type — PC-93 has since replaced that workaround with
a real fix on all three tools (`assess`, `simulate`, `assess_canvas`): supplying
`Tool.InputSchema`/`OutputSchema` directly, reflected via the same `invopop/jsonschema`
call `cmd/gen-contracts` already uses, so the SDK's own broken reflection path (which,
per its real source, is only invoked when those fields are left `nil`) never runs at
all. `core/`'s frozen contract tags were never weakened.

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
