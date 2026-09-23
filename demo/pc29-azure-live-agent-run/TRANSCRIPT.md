# Live-agent run: Azure — PC-29's "both clouds" criterion

This is a **live agent's real run** against a real, running `assessd`, on Azure — the
same discipline as `demo/pc28-live-agent-run/TRANSCRIPT.md`'s AWS run: the agent (this
session) called the real `/assess` HTTP endpoint, read each response's actual findings,
and decided what to fix from that evidence, not from a pre-scripted stub.

This run only became possible after fixing two real gaps found while building it:

1. **`core.BuildFindings` was hardcoded to AWS node IDs** — genericized to run its
   storage-encryption and RPO-feasibility checks against any `managed_database` node,
   reading the already-provider-agnostic `Capability.EncryptionMechanism`/
   `ReplicationMode` fields instead of an AWS-specific raw attribute key. AWS's own
   finding IDs/values are unchanged — verified with a dedicated regression test
   (`TestAWSFindingIDs_UnchangedAfterGenericization`).
2. **`server.Assess` hardcoded AWS's own provider registry** — so this exact call
   would have failed the Minimum Viable Graph check outright before either fix, with
   every Azure resource type unrecognized. Fixed by merging both providers' registries
   (`providers.Merge`) — resource_type strings are already provider-namespaced
   (`aws_*`/`azurerm_*`), so the merge is unambiguous and the caller never declares
   which cloud a bundle is.

## Setup

- Server: `assessd` (P1), rebuilt with both fixes above, listening on `:8099`.
- Bundle: a fresh working copy of `golden/azure-broken` at
  `demo/pc29-azure-live-agent-run/` (checked-in fixture never touched).
- Session: `pc29-azure-live-agent-demo`.
- Raw request/response JSON for both calls: `responses/v1.json`, `v2.json`.

## Run

### Version 1 — initial assessment

`POST /assess` against the untouched broken bundle. Two findings come back real and
actionable:

| Finding | Value | Evidence |
|---|---|---|
| `finding.compliance.sql-storage-encryption.azurerm_mssql_database.payments` | `unsatisfied` | "storage encryption is not enabled — data at rest, snapshots, and automated backups remain in plaintext..." (attribute: `transparent_data_encryption_enabled`) |
| `finding.compliance.sql-rpo-feasibility.azurerm_mssql_database.payments` | `not_assessable` | "replication mode \"none\" has no known RPO feasibility rule" |

(`zone-kill` and `nat-gateway-redundancy` correctly read `not_assessable` — both are
AWS-specific checks with no Azure equivalent in this bundle, a stated gap, not a bug;
see `golden/azure/README.md`.)

**Agent's decision:** both findings trace to the same resource,
`azurerm_mssql_database.payments`, and — reading the whole resource block, not just
one flagged field — both `zone_redundant` and `transparent_data_encryption_enabled`
are set to `false`. Both flipped to `true` in one edit.

### Version 2 — after the fix

`POST /assess` again, same session. Both findings converge to `satisfied` in **one
iteration** (Azure's two defects live on the same resource, unlike AWS's two separate
resources needing two rounds). Real `assurance_delta`:

```
finding.compliance.sql-storage-encryption...: improvement    (unsatisfied -> satisfied)
finding.compliance.sql-rpo-feasibility...:     resolved_risk  (not_assessable -> satisfied)
```

`resolved_risk` (not `improvement`) for the RPO finding — PC-83's fix, now proven on a
second cloud, not just AWS. `terraform validate` against the fixed working copy passes.

## What this is, and isn't

- **Is:** real, live evidence that the assurance loop works on Azure, not just AWS —
  PC-29's own "at least one full iteration cycle on each cloud" criterion, for the AWS
  side see `demo/pc28-live-agent-run/`.
- **Isn't:** the region-loss criterion (PC-82 + PC-23) or the publish criterion (PC-16,
  PC-26) — both separate, not attempted here.
- **Isn't** a claim that zone-kill or NAT-redundancy now work for Azure — those remain
  the stated, separate gap named in `golden/azure/README.md`.
