# Ingest silent-drop audit (PC-158)

**Rule (I4, and the PRD's own):** a construct the engine cannot read surfaces as `unresolved` / `not_assessable`
with a reason. It is never dropped, and never read as a documented default. "The engine found nothing wrong"
must never mean "the engine never saw it".

PC-157 broke that rule (a policy written as `jsonencode()` was dropped, so a role "satisfied" least privilege).
This audit looks for the rest of the class, and a value-level mutation sweep keeps it from coming back.

## How an unreadable value is now handled

Ingest classifies every declared attribute as **literal**, **pure references** or **other** (`ingest.AttrInfo`).
Where the engine reads an attribute (data: `unresolved_matters` in `providers/aws/*.yaml`), a rule inside a
security group, route table or network ACL, an association, or a policy, a declaration ingest cannot read is
recorded on the node as `unresolved_inputs: "<affects>: <detail>"` (`core/unresolved.go`). The consumer that
depends on it withdraws its verdict. Nothing is defaulted.

## Findings

Found by the sweep (S) or by reading the code (R). Each has a regression test.

| # | Path | What was silently wrong | Fix |
|---|---|---|---|
| 1 | IAM identity policy as `jsonencode()` (S, PC-157) | dropped; role "satisfied" | evaluate `jsonencode` of constants; an unread policy makes IAM evaluation not_assessable |
| 2 | IAM attachment with an unreadable `role` (S) | attached to nothing; every role unaffected | every role records it |
| 3 | Security-group rule fields, direction, owner (S) | field dropped or rule attached nowhere; verdict flipped | SG marked incomplete; trace and compliance not_assessable. Unmodelled sources (`self`, IPv6, prefix lists) flagged too |
| 4 | Security-group attachment unreadable (S) | no edge | profile incomplete |
| 5 | Placement (`subnet_id`, `subnets`, `subnet_ids`, `*_subnet_group_name`) (S) | edge dropped; AZ-loss and NAT-redundancy findings computed without it | those findings not_assessable |
| 6 | Route table association / route entries (S, K) | route table stopped being a route table; the trace silently used a sibling subnet | unresolved routes_to edge keeps the table's identity; marker; egress not_assessable |
| 7 | `tags` unreadable (S) | the public-tier check silently did not run | not_assessable finding |
| 8 | `aws_lb.internal` unreadable (S) | assumed internet-facing | not_assessable |
| 9 | NAT-sharing finding with an unreadable route (K) | finding vanished | not_assessable |
| 10 | **NACL inline rules used `rule_number`/`rule_action` (R)** | real inline blocks use `rule_no`/`action`: every inline rule was ingested with no number and no action (the PC-149 fixture used the wrong names too, which hid it) | correct per provider docs; fixtures fixed |
| 11 | **NACL inline `subnet_ids` association (R)** | emitted `contained_in NACL -> subnet`, never recognised; a NACL that **denied everything read as allowing everything** (assumed default) | recognised as the association |
| 12 | NACL rule fields, direction, owner, unmodelled ICMP/IPv6 (R) | same as 3 | NACL marked incomplete |
| 13 | S3 bucket policy unreadable (K) | dropped; an explicit **Deny** vanished and a denied request became **allowed** | not_assessable |
| 14 | RDS `multi_az` unreadable (R) | priced at the Single-AZ rate (about 2x low) | cost_unknown |
| 15 | Ambiguous CIDR in a SG/NACL rule (K) | evaluator said "not_assessable" only in a reason string; the trace turned `Allowed=false` into a **deny** | `NotAssessable` is a real field; trace maps it |

S = value-level sweep, K = kitchen-sink sweep, R = code review.

## Justified, not changed

* **Capabilities** (`storage_encrypted`, `multi_az`, ...): an unreadable value leaves the field nil, which every
  consumer already reports as not_assessable. The sweep confirms it.
* **Sizing**: an unparseable value becomes nil, so `cost_unknown`. Canvas the same.
* **Load balancer registration (PC-150)**: attachments-only by design; an unlisted target is not_assessable.
* **Zone-kill findings** use hard-coded golden subnet IDs (PC-28); their placement guard is generic, the findings are not.

## Known gaps (not fixed here)

* **Azure ingest** declares no `unresolved_matters`, so unreadable Azure relationships are not tracked.
* **Canvas capability strings** that should be boolean (`"true"`/`"false"`) are not domain-validated; anything else
  reads as "not true".
* **Two-level nested blocks** (a block inside a block) are not classified.
* A policy that references a resource ARN is not assessable at all (PC-159).

## The sweep (`tests/valuemutation`)

For each bundle (golden/aws, golden/aws-broken, and `testdata/kitchen`, a synthetic design using every construct
golden does not), every attribute of every resource and nested block is rewritten one at a time to an unreadable
expression (unknown function, unresolved reference, partly unreadable, malformed JSON for documents). Ingest and every
assessment are re-run (findings, PCI/SOC 2/CIS results, a trace per journey hop, IAM probes) and compared with the
baseline: an unreadable value may make a result `not_assessable` or leave it alone, never flip it or make it vanish.
About 2,350 mutations run in about 20 seconds as part of `go test ./...`.

A mutation sweep alone cannot see a bug that blinds the *baseline* (that is exactly what PC-157 was), so it is paired with:

1. **Seeded defects must be detected as they stand** (golden/aws-broken's wildcard role, encryption, NAT).
2. **Declared dependencies must go not_assessable**, not merely stay unchanged.
3. **Negative control:** an engine that ignores the unreadable-input records is run through the same sweep and must fail
   (`TestSweep_NegativeControl_*`).
