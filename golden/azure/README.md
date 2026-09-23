# Golden reference architecture — Azure (PC-22, PC-29)

The same payments workload as `golden/aws`, on Azure — the first real test of whether
the two-level IR abstraction genuinely generalizes beyond AWS, not just asserts that it
does. Both a clean bundle and a broken variant (`golden/azure-broken`, PC-29) now
exist — see "Real findings" below for the two additional gaps PC-29 found and fixed
(`BuildFindings` and `server.Assess` were both hardcoded to AWS) before a broken
variant could serve any real purpose.

```
golden/azure/
├── network.tf              resource group, VNet, 2 subnets, NSG, public IP
├── dns.tf                  Azure DNS zone + A record        (golden node 1/8: dns)
├── application_gateway.tf  App Gateway + its WAF policy     (golden node 3/8: load_balancer,
│                                                              plus its WAF policy: network_boundary)
├── aks.tf                  AKS cluster                      (golden node 4/8: container_workload)
├── sql.tf                  Azure SQL server + database      (golden node 5/8: managed_database)
├── cache.tf                Azure Cache for Redis            (golden node 6/8: cache)
├── queue.tf                Service Bus namespace + queue    (golden node 7/8: queue/stream)
├── rbac.tf                 RBAC role assignment             (golden node 8/8: identity)
├── variables.tf, versions.tf
```

## Status

| | |
|---|---|
| `terraform validate` | **passing**, Terraform v1.15+, azurerm provider ~> 4.0 |
| All 8 golden node types mapped and present | **yes** — `providers/azure/*.yaml`, verified via `ingest/azure_golden_test.go` |
| IR/`CapabilityModel` generalization proven end-to-end | **yes** — `ingest.Ingest` runs unmodified against this bundle; `azurerm_mssql_database.payments` correctly derives `replication_mode: sync` from `zone_redundant`, through the same failover engine AWS uses |
| Comparable compliance findings (`core.BuildFindings`) | **yes, as of PC-29** — genericized to run storage-encryption/RPO-feasibility checks against any `managed_database` node; AWS's own finding IDs/values stayed byte-identical (regression-tested) |
| `server.Assess` works against an Azure bundle | **yes, as of PC-29** — it hardcoded AWS's provider registry until fixed (`providers.Merge`); see `server/azure_assess_test.go` |
| A real, live-agent iteration cycle recorded | **yes** — `demo/pc29-azure-live-agent-run/TRANSCRIPT.md`: converges in 1 round, `resolved_risk`/`improvement` correctly classified (PC-83's fix, proven on a second cloud) |
| AZ-loss / zone-kill scenario | **still not applicable to this bundle — a real model difference, not a gap to force closed** (see below) |
| Broken variant (`golden/azure-broken`) | **built (PC-29)** — 2 seeded defects on `azurerm_mssql_database.payments`, mirroring `golden/aws-broken`'s defect 2 shape |

## Real findings from building this, not assumptions carried over from AWS

**Azure has no subnet-level AZ placement.** Verified against
`terraform-provider-azurerm`'s own `azurerm_subnet` docs: it has no zone argument at
all. AWS ties AZ placement to the subnet (`availability_zone`); Azure's VNets/subnets
are regional constructs spanning every zone, and zone-redundancy is expressed
per-*resource* instead (`azurerm_mssql_database.zone_redundant`, an Application
Gateway's own zone spanning, an AKS node pool's `zones`, a Public IP's `zones`). PC-14's
zone-kill engine (`core.ContainmentBlastRadius` over `contained_in` edges, killing a
subnet node) has nothing to operate on here — there is no per-AZ subnet to kill. Faking
one would misrepresent real Azure architecture; building a resource-level zone-kill
engine instead is real `core/internal/analyse` work, explicitly out of PC-22's own
scope ("mapped (YAML data + loader)"), confirmed with the user before proceeding rather
than assumed. **A real, separate follow-up ticket, not silently skipped.**

**`core.BuildFindings` did not generalize to Azure — fixed under PC-29.** It was
hardcoded to specific AWS node IDs (`aws_db_instance.payments`, `aws_subnet.public_a`,
etc.), and the RDS storage-encryption check read `RawAttributes["storage_encrypted"]`
— an AWS-specific raw key name, not the already-provider-agnostic
`CapabilityModel.EncryptionMechanism` field the failover engine already used. Fixed by
looping over every `managed_database` node in the IR (any provider) and reading the
canonical `Capability.EncryptionMechanism`/`ReplicationMode` fields generically — see
`core/findings_builder.go`'s `managedDatabaseLabel` for how AWS's own already-tested
finding IDs (`rds-storage-encryption`, `rds-rpo-feasibility`) stayed byte-identical
while Azure SQL got its own honest label (`sql-storage-encryption`,
`sql-rpo-feasibility`) rather than either inheriting "rds-" or a meaningless generic
fallback. Regression-tested (`TestAWSFindingIDs_UnchangedAfterGenericization`) and
verified live against the real server.

**`server.Assess` also hardcoded AWS's own provider registry — also fixed under
PC-29.** Even with `BuildFindings` genericized, the real HTTP endpoint would have
failed the Minimum Viable Graph check outright against this bundle — every Azure
resource type was unrecognized, since only `providers/aws`'s registry was ever loaded.
Fixed with `providers.Merge`, combining both providers' registries into one (safe and
unambiguous — resource_type strings are already namespaced by provider prefix,
`aws_*`/`azurerm_*`) rather than adding a provider-selection field to the request. This
was found live, not in a unit test — the first real `/assess` call against this bundle
failed with exactly this error, before the fix.

**Network substrate (VNet, subnets, NSG, public IP) is deliberately unmapped** —
`providers/azure` has no equivalent of `providers/aws`'s `vpc.yaml`/`subnet.yaml`/
`security_group.yaml`/`nat_gateway.yaml`. This mirrors AWS's own history exactly: those
AWS mappings were PC-78, a separate ticket from PC-13's original 8 — and PC-78's own
motivating reason (giving PC-14's zone-kill engine real per-AZ nodes) doesn't apply to
Azure at all per the finding above. Not an oversight; the same scope split AWS already
has, applied consistently.

**`dns` and `load_balancer` are structurally disconnected nodes in this bundle's IR.**
Verified against the real `azurerm_dns_a_record` docs: its `target_resource_id`
example aliases a Public IP resource, not an Application Gateway directly. Since Public
IP isn't one of the golden 8 (matching AWS's own unmapped `aws_eip`/
`aws_internet_gateway`), the reference chain `dns -> [public IP, unmapped] ->
load_balancer` produces no edge — `ingest` only forms an edge when both ends have a
real mapped node. A genuine one-hop-vs-two-hop topological difference from AWS's Route
53 alias block (which references an ALB directly), not a bug.

**Front Door is out of scope**, in favor of Application Gateway for both
`load_balancer` and its attached WAF policy — see
`providers/azure/application_gateway.yaml`'s own doc comment for the explicit decision
and why modeling both would have meant a full Front Door profile/endpoint/origin/route
chain, not a simple substitution.

**Two real, doc-verified polarity bugs were caught before they shipped**, not after:
AKS's `private_cluster_enabled` and Azure Cache for Redis's `non_ssl_port_enabled` both
default to `false`, but naming either field to match `assignCapabilityField`'s
recognized names (`control_plane_public_access`, `transit_encryption_enabled`) while
storing the un-inverted raw value would have silently flipped the semantic meaning —
see `providers/azure/aks.yaml` and `providers/azure/redis_cache.yaml` for the full
account of each.

## The broken variant (`golden/azure-broken`, PC-29)

Now built, since `BuildFindings` and `server.Assess` both actually work against Azure.
Two seeded defects on `azurerm_mssql_database.payments` — `zone_redundant = false` and
`transparent_data_encryption_enabled = false` — mirroring `golden/aws-broken/rds.tf`'s
defect 2 shape exactly. `terraform validate` passes. The real delta between
`golden/azure-broken` and `golden/azure` is hand-verified in
`core/findings_builder_azure_golden_test.go`'s
`TestAssuranceDelta_AgainstGoldenAzureBundles`, and demonstrated live end-to-end in
`demo/pc29-azure-live-agent-run/`.
