# Golden reference architecture

The single payments workload from CLAUDE.md §14, authored as real Terraform. Every other
story in Phase 1-3 validates against this bundle, which is why PC-15 is called out in
§20 as the highest-leverage early task rather than a late one.

```
golden/
├── workload.yaml   declared NFRs (tier1, PCI, 99.95%, RTO 60s, RPO 0)
├── aws/            clean baseline - the target state
└── aws-broken/     deliberately-imperfect variant - the starting state for PC-28
```

## Status

| | |
|---|---|
| `terraform validate` (both variants) | **passing**, Terraform v1.15.8, AWS provider ~> 5.0 |
| Eight golden node types covered | **yes**, both variants |
| Golden-file fixtures (expected IR, findings, simulations) | **not yet** - blocked on PC-12 and PC-13 |
| `terraform plan`/`apply` against a real account | **not attempted** - no credentials used |

Neither variant has been applied. `validate` confirms the configuration is internally
consistent and provider-schema-correct; it does not confirm AWS would accept it. Rung 3
(PC-25) is where that gets proven.

## The eight node types

Per §14, coverage is capability-based, not resource-count-based. Each type appears once,
in its own file:

| Capability | Resource | File |
|---|---|---|
| `dns` | `aws_route53_zone` / `_record` | `dns.tf` |
| `waf` | `aws_wafv2_web_acl` | `waf.tf` |
| `load_balancer` | `aws_lb` | `alb.tf` |
| `container_workload` | `aws_eks_cluster` / `_node_group` | `eks.tf` |
| `managed_database` | `aws_db_instance` | `rds.tf` |
| `cache` | `aws_elasticache_replication_group` | `cache.tf` |
| `queue` | `aws_sqs_queue` | `queue.tf` |
| `identity` | `aws_iam_role` / `_policy` | `iam.tf` |

`network.tf` and `security.tf` are substrate, not one of the eight golden node types —
they exist because a `load_balancer` or `managed_database` with no subnet or AZ identity
cannot be reasoned about for AZ loss. **As of PC-78, this substrate IS mapped into the
IR** (`providers/aws/{vpc,subnet,nat_gateway,security_group}.yaml`, all `network_boundary`
nodes with real `contained_in` placement) — it was Terraform from the start (PC-15), but
had no provider mapping until PC-78 closed that gap, which is what made PC-14's
zone-loss engine's real-bundle result vacuous until then.

## Defect inventory (aws-broken)

Eight defects, each with a header comment in its file stating the expected finding. The
variant is branched from the clean baseline by targeted patch, so `diff -r aws aws-broken`
is itself a fixture — it is what PC-19's version diffing and Assurance Delta consume.

| # | Defect | Kind | File |
|---|---|---|---|
| 1 | One NAT gateway serves all three private subnets | SPOF, tier1 | `network.tf` |
| 2 | Database single-AZ **and** storage unencrypted | SPOF + RPO + PCI | `rds.tf` |
| 3 | Single cache node, no failover, no encryption | SPOF + PCI | `cache.tf` |
| 4 | Settlement queue has no DLQ and no CMK | durability + PCI | `queue.tf` |
| 5 | Workload single-replica single-AZ, public control plane | SPOF + PCI | `eks.tf` |
| 6 | Web ACL exists but is never associated with the ALB | compliance, silent | `waf.tf` |
| 7 | Application role is `Action: "*"`, `Resource: "*"` | identity, tier1 | `iam.tf` |
| 8 | No DNS health check, target health not evaluated | recovery | `dns.tf` |

Defect 6 is the one worth keeping: the web ACL is individually correct and would pass a
resource-by-resource review. Only the *absent edge* between `web_acl` and `load_balancer`
reveals it. A finding that requires the graph is the clearest justification for building
an IR at all.

Defects 2, 3 and 4 also leave the customer-managed KMS key with no consumers — an unused
CMK is a legitimate derived finding, not an oversight in the fixture.

## Two decisions recorded here

**The broken variant is a separate directory, not a flag on the baseline.** PC-15's
Conversation asks whether it should be one bundle or a branched variant. Separate, because
PC-28's acceptance test needs to *mutate* the broken bundle toward the clean one across
iterations — that requires two independently-parseable bundles and a diff between them. A
single bundle behind a toggle would make "what changed this iteration" a property of
Terraform variables rather than of the IR, which is the wrong layer for the Assurance
Delta to read.

**Fixtures are not hand-written.** PC-15's Conversation is explicit: derive them from a
first correct run, then hand-verify. So `fixtures/` is deliberately absent until PC-12
(parser) and PC-13 (AWS mappings) land. Writing expected IR by hand now would produce a
fixture that encodes my assumptions about the IR instead of testing the parser against it.

## Parser-subset boundary (§15)

The bundle is authored inside the v1 parsed subset — resource/data blocks, variables,
locals, static references. No `count`, no `for_each`, no `dynamic` blocks. Three AZs are
written out explicitly rather than derived, which is verbose on purpose: this bundle is
what the parser is measured against, so it should not contain constructs the defined
subset cannot resolve.

One deliberate exception, marked in `queue.tf`: `redrive_policy` uses `jsonencode()` with
a live reference to the DLQ. There is no way to express a redrive policy without encoding
JSON, and hardcoding the ARN would erase the `queue -> dead_letter_queue` edge the failure
model needs. PC-12 must either extract the reference from inside the function call or
surface it as `unresolved` — never drop the edge silently. IAM policy documents in
`iam.tf` use the same pattern for the same reason.

## Not yet decided

- `environment` (dev/staging/prod) is a plain tag here. Whether it becomes a first-class
  dimension of `workload.yaml` is an open question in §20 and gates Terragrunt-style
  export work. Not assumed either way.
- The CIDR/subnet floor for the Minimum Viable Graph check (§15) needs tuning against real
  fragments once the parser exists. This bundle is comfortably above any plausible floor
  (1 entry point, 2 stateful nodes), so it cannot be used to calibrate it.
- The Azure side of §14 is not authored. PC-22 owns it.
