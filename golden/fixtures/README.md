# Golden fixtures

Derived, not hand-written — PC-15's own Conversation is explicit: "don't hand-write
expected IR/findings, derive them from a first correct run and then hand-verify." These
files are the output of `go run ./cmd/gen-golden-fixtures` run against `golden/aws`,
`golden/aws-broken`, `golden/azure`, and `golden/azure-broken` (PC-22, PC-29), generated
once the parser (PC-12) and provider mappings (PC-13/PC-22) existed to derive them
from, then hand-verified against the defect inventory in `golden/README.md` (AWS) /
`golden/azure/README.md` (Azure).

**`azure.findings.json`/`azure-broken.findings.json` now carry real, comparable
findings (PC-29)**, not the all-`not_assessable` placeholder they briefly did while
`core.BuildFindings` was still hardcoded to AWS node IDs. Genericized to run against
any `managed_database` node — see `golden/azure/README.md` for the full account,
including a second, independent gap found and fixed alongside it (`server.Assess` also
hardcoded AWS's own provider registry). `azurerm_mssql_database.payments` reads
`satisfied`/`satisfied` in `azure.findings.json` and `unsatisfied`/`not_assessable` in
`azure-broken.findings.json`, with the real delta between them
(`improvement`/`resolved_risk`) hand-verified in
`core/findings_builder_azure_golden_test.go` and demonstrated live in
`demo/pc29-azure-live-agent-run/`.

## Regenerating

```bash
go run ./cmd/gen-golden-fixtures
```

Regenerate whenever `golden/aws`, `golden/aws-broken`, `ingest/`, or `providers/aws/`
changes. If a regeneration changes these files unexpectedly, that is a signal to
hand-verify the new output against the defect inventory again before committing it —
these files are checked-in output, not a cache to trust blindly.

## What's here, and what isn't

| | Status |
|---|---|
| `aws.ir.json`, `aws-broken.ir.json` | **Present** — real IR, derived from the actual parser/mapping pipeline |
| `aws.findings.json`, `aws-broken.findings.json` | **Present** (PC-14/PC-17/PC-18/PC-28) — 5 representative findings per bundle: two zone-kill `FailureMode`s and three compliance `Finding`s, derived from the real engines, not hand-written |

PC-15's fixture criterion asked for IR **and** findings/simulations. Both halves now
exist. The findings half is **deliberately narrow, not exhaustive** — five specific,
hand-verified scenarios per bundle, matching this project's own "narrow and correct
over broad and shallow" philosophy — not every finding this architecture could
possibly produce. Extending coverage (more controls, more zone-kill scenarios, a real
SPOF-via-min-cut finding) is ordinary future work, not a gap in what this ticket
promised.

### The five findings, and why each was chosen

Two findings were added under PC-28, after driving the agent-iteration acceptance
harness (`server/agent_iteration_acceptance_test.go`) off the original three findings
revealed that only `finding.compliance.rds-storage-encryption...` was actually
delta-visible between the broken and clean golden bundles — the two zone-kill findings
either didn't differ between bundles at all, or used a free-form `Outcome` value
`ComputeDelta` can't rank by design (PC-19's own stated scope boundary). Per PC-28's
Card ("treat any agent failure to converge as a signal to fix the output shape, not the
agent"), this is that fix:

- **`finding.compliance.nat-gateway-redundancy`** (PC-28) — a pure topology check, no
  new AWS-doc verification needed: does every declared public subnet retain independent
  NAT gateway coverage? `unsatisfied` in `aws-broken.findings.json` (names
  `aws_subnet.public_b`/`aws_subnet.public_c` as uncovered — defect 1's exact shape:
  only `nat_a` remains), `satisfied` in `aws.findings.json` (one NAT gateway per AZ).
  Verified end-to-end in `core/findings_builder_golden_test.go`'s
  `TestNATRedundancyFinding_AgainstGoldenBundles`.
- **`finding.compliance.rds-rpo-feasibility.aws_db_instance.payments`** (PC-28) —
  directly checks the ONE declared hard requirement this codebase has a real engine for
  (`workload.yaml`'s `rpo_seconds: 0`), reusing PC-14's previously-unwired
  `RPOFeasibility`/`DeriveNodeReplicationAndFailover`. `not_assessable` in
  `aws-broken.findings.json` (`multi_az = false` means replication mode `"none"`, which
  has no RPO feasibility rule — PC-14's own deliberate design, not relitigated here) and
  `satisfied` in `aws.findings.json` (`multi_az = true` gives synchronous replication,
  feasible against any declared RPO including 0). Note: because one side is
  `not_assessable`, `ComputeDelta` correctly classifies this finding's own delta entry
  as `not_assessable` rather than `improvement` when comparing the two versions — "an
  unknown state compared against a known one is itself unknown" (PC-19's own rule, not
  changed here). Verified end-to-end in
  `TestRDSRPOFeasibilityFinding_AgainstGoldenBundles`.

The original three, unchanged:

- **`finding.zone-kill.public-a`** — a real, non-empty zone-kill result (`core.
  ContainmentBlastRadius` + `core.DeriveImpact`, PC-14/PC-78). The positive control:
  proves a finding CAN be confidently assessed, not everything is unknown.
- **`finding.zone-kill.data-a`** — used to be the honest not_assessable example:
  RDS/ElastiCache's own subnet placement wasn't visible to zone-kill (their
  `aws_db_subnet_group`/`aws_elasticache_subnet_group` indirection was unmapped).
  PC-80 closed that gap (both resources now map to `network_boundary` with
  `reference_edge_type: contained_in`, the same pattern `aws_subnet` itself uses), so
  this finding is now a real, non-empty, `assessed` result in both bundles — `4
  component(s) affected`, checked end-to-end (not just by reading source) against
  the actual regenerated fixture. See `cmd/gen-golden-fixtures/main_test.go`'s
  `TestFixtureFindings_ZoneKillDataA_IsAssessed` and `core/zoneloss_golden_test.go`'s
  `TestZoneKill_CleanBundle_DataA_ExactBlastRadius` for the hand-worked blast radius
  this value is derived from.
- **`finding.compliance.rds-storage-encryption...`** — PC-18's one control, differing
  correctly by bundle (`satisfied` clean, `unsatisfied` broken/defect 2).

## Hand-verification record

**Updated after PC-22's failover relocation** (`aws.ir.json`/`aws-broken.ir.json`, node
counts unchanged): `aws_db_instance.payments` and `aws_elasticache_replication_group.
settlement` now carry real `capability.replication_mode`/`failover_mechanism` values —
previously always absent, since nothing populated `CapabilityModel`'s `ReplicationMode`/
`FailoverMechanism` fields before this. Confirmed against the raw Terraform, not
assumed: `aws.ir.json` — RDS `sync`/"Multi-AZ automatic failover to a synchronous
standby replica" (`golden/aws/rds.tf`'s `multi_az = true`), ElastiCache `async`/
"automatic promotion of a replica to primary on primary failure" (`golden/aws/cache.tf`'s
`automatic_failover_enabled = true`); `aws-broken.ir.json` — both `none`/`none` (RDS's
`multi_az = false` is defect 2; ElastiCache's `automatic_failover_enabled = false` in
`golden/aws-broken/cache.tf`, confirmed via `grep` against both files' real content, not
inferred from the defect-comment headers alone). This relocation moved the derivation
logic itself from `core/internal/analyse` (provider-agnostic, wrongly held
AWS-specific wording) to `providers/aws`'s own `FailoverMapping` data + `ingest/
build.go`'s `deriveFailover` — see that function's own doc comment.

**Updated after PC-78** (network substrate — VPC/subnet/NAT/security group — added to
`providers/aws`'s mapping scope): **28 nodes / 33 edges** in `aws.ir.json`; **25 nodes /
28 edges** in `aws-broken.ir.json`. The 3-node difference is exactly `aws_nat_gateway.
nat_b`, `aws_nat_gateway.nat_c` (defect 1: only one NAT gateway remains) and
`aws_sqs_queue.settlement_dlq` (defect 4: no DLQ) — confirmed by diffing the two node
ID sets directly, not by assuming the counts meant what they appeared to.

(Counts have moved further since: PC-80 added the RDS/ElastiCache subnet-group
substrate, and PC-111 added route tables/IGW/NAT-gateway routing — `aws.ir.json` is now
35 nodes / 59 edges, `aws-broken.ir.json` 32 nodes / 52 edges. Recorded here rather than
silently updating the PC-78 numbers above, which describe what PC-78 itself actually
produced at the time — see `git log` on this file, or `go run ./cmd/gen-golden-fixtures`'s
own stdout, for the current real counts going forward.)

Original record, still accurate for the golden-8 node types and their edges:

- The edge differences include defects 4 and 6: the `settlement -> settlement_dlq`
  edge disappears (no DLQ to reference) and the `aws_lb.payments -> aws_wafv2_web_acl.
  payments` association edge disappears (defect 6: WAF never associated).
- `aws_db_instance.payments`'s capability fields differ exactly as defect 2 intends:
  `multi_az_implementation` and `encryption_mechanism` are `"true"`/`"true"` in the clean
  bundle and `"false"`/`"false"` in the broken one; `backup_semantics` is `"14"` vs `"1"`.

Two real bugs were caught by hand-verification, not by tests written in advance:

1. (PC-12) The first generated `aws.ir.json` contained two edges FROM
   `aws_wafv2_web_acl_association.payments` — a resource that (correctly) produces no
   node of its own, meaning those edges pointed from an ID that didn't exist anywhere
   in `IR.Nodes`. Fixed in `ingest/build.go`'s `buildEdges`, now covered by a permanent
   structural test.
2. (PC-14, caught via `core/failover_golden_test.go` against `golden/aws-broken`
   specifically) `RTOFeasibility` did an exact-string match against a sentinel that its
   own producer function didn't actually return exactly — silently reporting a
   failover path existed for a database that had none. Fixed with a single shared
   `FailoverMechanismNone` constant (`core/internal/analyse/failover.go`).

Both are exactly the value of "derive, then hand-verify" over trusting generated output
by construction — see PC-12's and PC-14's Jira history for the full account of each.

## `aws.report.json` / `aws.report.html` (PC-120/PC-122)

Golden/aws only, deliberately — a projection over the same IR/findings this generator
already produces above, not a second independent thing to keep four bundles' worth of
in sync. `aws.report.json` is `core.BuildReport`'s own output (no pricing snapshot
exists in this generator, so `cost.available` is honestly `false`); `aws.report.html`
is that same report through `core.RenderReportHTML`, including the real, live-rendered
PC-81 SVG diagram embedded directly (`Report.Graph`) — both byte-compared in CI
(`core/report_golden_test.go`, `core/report_html_golden_test.go`), each with its own
negative control. There is no `aws.report.pdf` fixture: PC-122's own PDF export
(`render/pdf.go`, headless Chromium) is NOT claimed byte-stable across machines — its
dates are normalized so two renders in one environment are identical, but PDF bytes
also depend on the Chromium build and installed fonts — see that file's own doc comment
for the full determinism decision. The PDF is a print of `aws.report.html`. Regenerating: same
`go run ./cmd/gen-golden-fixtures` command as above; requires a real `dot` (Graphviz)
on PATH to render the diagram, exactly like every other command that touches this
directory already does via `render.SVG`.
