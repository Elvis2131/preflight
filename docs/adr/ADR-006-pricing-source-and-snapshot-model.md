# ADR-006: Pricing source and snapshot model

- **Status:** Accepted
- **Date decided:** 2026-09-25
- **Ratified into this log:** 2026-09-25 (PC-116)
- **Provenance:** written directly against the PC-116 issue record (Card, Conversation,
  Confirmation), verified against AWS's own real, live Price List API before any code
  was written.

## Context

The cost engine (PC-117/118) needs current AWS prices, but an assessment must stay
reproducible — the same IR + workload, assessed twice, must produce the same cost,
even if AWS's real prices moved in between. That requires pricing captured as dated,
versioned snapshots, computed **outside** the assessment path, never fetched live
during an `/assess` call (NFR-1, I1, NFR-15: the assessment engine must never make an
external network call at all).

## Decision

### 1. Bulk API, not Query API

AWS publishes two pricing interfaces:

- **The Bulk API** (`pricing.us-east-1.amazonaws.com/offers/v1.0/aws/...`) — plain
  HTTPS GET of JSON files, **no AWS account or credentials required at all**, verified
  directly against the live endpoint while writing this ADR (`AmazonElastiCache`'s
  `region_index.json`, `AWSELB`'s and `AmazonS3`'s region index files all fetched
  successfully with a bare `curl`, no auth headers). It retains historical price-list
  versions (each region index lists a dated `currentVersionUrl` like
  `.../20260914063714/us-east-1/index.json`), which is exactly the "dated snapshot"
  shape this ticket needs.
- **The Query API** (`pricing:GetProducts`, part of the AWS SDK) — current prices
  only, no history of its own, and requires a SigV4-signed call with a real IAM
  identity.

**Bulk wins**: it already IS a snapshot model (AWS's own dated version URLs), and it
needs no credentials for the services this ticket scopes to (see §3). The Query API's
"lighter for a narrow service set" advantage does not offset losing AWS's own
retained history and, more importantly, does not offset needing real IAM credentials
for a job that otherwise needs none.

**Real, verified caveat**: Bulk API region files are not uniformly small.
`AWSELB`/`us-east-1` is 19KB; `AmazonS3`/`us-east-1` is 473KB; `AmazonRDS`/`us-east-1`
is 27MB; `AmazonEC2`/`us-east-1` is **481MB** (`curl -I`, verified 2026-09-25). A
fetcher must stay small-file-only for v1, and EC2 (and NAT Gateway, which is a
`productFamily` inside the same EC2 offer file) is deliberately **deferred past v1** on
cost/practicality grounds — see §3.

### 2. Credential placement: no separate process needed, because none is needed at all

The Card's own framing ("both APIs require an IAM identity") is **not accurate for the
Bulk API** — recorded as a correction, not silently reproduced. Since the fetcher makes
zero AWS-authenticated calls, ADR-003's credential boundary (P3 = `cmd/runnerd`, the
only process ever permitted to hold cloud credentials) is not actually load-bearing
for *credentials* here.

It still matters for the **network-call boundary** (NFR-1/I1: the assessment engine,
P1, must never make an external network call, credentialed or not). The fetcher
therefore lives in `cmd/runnerd/internal/pricingfetch` — compiler-enforced private to
`cmd/runnerd` (Go's `internal/` visibility rule), the same structural guarantee
`cmd/runnerd/internal/creds` already established for PC-10, extended here to cover any
outbound network call, not just a credentialed one. `cmd/assessd`/`cmd/reasond` are
proven to have zero import path to it (`cmd/runnerd/internal/creds/boundary_test.go`'s
own pattern, extended).

Chosen over "a separate scheduled job" (the Card's other option): a fourth
long-running process would need its own justification against ADR-003's three-process
topology; a one-shot subcommand of the already-existing P3 binary
(`runnerd -fetch-pricing`), invoked on a cron cadence, gets the same "runs outside the
assessment path, on its own schedule" property without adding a process.

### 3. Snapshot scope, v1

**PC-132 usage-type verification (2026-10-01).** A one-off anonymous read of the
public `current/us-east-1/index.json` files confirmed representative rows for
`NatGateway-Bytes` (AmazonEC2), `DataTransfer-Out-Bytes` and
`DataTransfer-Regional-Bytes` (AWSDataTransfer), and `LCUUsage` with
`operation=LoadBalancing:Application` (AWSELB). The compact capture manifest is
`cmd/runnerd/internal/pricingfetch/testdata/pc132_usage_type_rows.json`. This
verifies matcher vocabulary; it does not change the deliberate v1 decision to
defer fetching the 481MB EC2 offer file or NAT Gateway pricing.

- **Services fetched for real, v1**: `AWSELB` (load balancers), `AmazonS3`,
  `AmazonElastiCache`, `AmazonRDS` — all four verified reachable and parseable against
  the live endpoint while writing this ADR. RDS's 27MB file is decoded fully in
  process, then filtered down to the requested instance classes before being
  normalized into the snapshot — acceptable at this size (real, measured: well under
  what a one-shot CLI fetch can hold comfortably); this is NOT the same claim as
  "streamed" and is not expected to scale to EC2's 481MB (see the deferral below and
  the Revisit trigger).
- **Deferred past v1, stated not silent**: `AmazonEC2` and NAT Gateway pricing (a
  `productFamily` within the same EC2 offer file). The real, measured 481MB/region
  file size makes an in-process, unbatched fetch impractical within this ticket's
  scope; a real EC2 fetcher needs either per-instance-type targeted retrieval (the
  Bulk API does not support server-side filtering, so this means a different
  strategy — e.g. the Query API for this one service, or downloading and indexing the
  file out-of-band) that is real, additional design work, not a one-line addition to
  the fetcher built here.
- **Regions**: `us-east-1` only for v1 (matches the golden bundle's own declared
  region).
- **On-demand only**, no Reserved/Savings Plans pricing, per the Card.

### 4. Snapshot shape and storage

A snapshot is: a snapshot ID, a `fetched_at` timestamp, a `source` (API + the real AWS
price-list version string captured from each service's own dated URL), AWS's own
disclaimer text (carried as metadata — "AWS states list prices are informational and
the service pricing page is what's charged," never omitted from a report), and a flat
table of normalized entries: `(service, region, sku_attributes, unit, price,
currency)`.

Stored in its own SQLite-backed store (`pricing.Store`, `pricing/store.go`) — a
separate schema/concern from `server.Store`'s session/version tables, but the same
storage technology (ADR-002) and the same WAL-mode discipline. `pricing` itself
contains zero network code (verified structurally, see Consequences) so it is safely
importable by `server` (to serve the read endpoints) without pulling any network
capability into P1.

### 5. Endpoints

`GET /pricing/snapshots` (list), `GET /pricing/snapshots/{id}` (metadata + entries),
`POST /pricing/snapshots/{id}/activate` (mark the active default) — all served by
`assessd` (P1), reading `pricing.Store` directly. None of these make an AWS call; they
only read/write already-fetched, already-normalized local data.

## Rejected alternatives

- **Query API (`pricing:GetProducts`).** Rejected: needs real IAM credentials for a
  job that, using Bulk instead, needs none; no built-in historical versioning of its
  own (see §1).
- **A separate, fourth long-running scheduled-job process.** Rejected in favor of a
  one-shot `cmd/runnerd` subcommand (see §2) — same "outside the assessment path, own
  cadence" property without adding a process to ADR-003's topology.
- **Fetching the full EC2 offer file for v1.** Rejected on real, measured size (481MB
  for one region) — see §3.

## Consequences

- `pricing/` (types + SQLite store) has zero network-capable imports — a structural
  test (`pricing/boundary_test.go`) parses its own source and confirms no `net/http`
  client-call site and no AWS SDK import.
- `cmd/runnerd/internal/pricingfetch` is the only package that ever calls the real AWS
  Bulk API — proven both by Go's own `internal/` compiler rule (an outside import fails
  to build) and by an explicit dependency-graph check
  (`cmd/runnerd/internal/creds/boundary_test.go`'s own
  `TestP1AndP2HaveZeroCloudSDKOrValidateImports`, extended to also forbid
  `preflight/cmd/runnerd/internal/pricingfetch`).
- EC2 and NAT Gateway pricing remain `cost_unknown` until a follow-up ticket builds a
  real, targeted fetch strategy for them — not silently absent, a stated gap.

## Revisit trigger

Reopen if EC2/NAT Gateway pricing becomes a blocking product need — the "decode fully
in process" strategy this ADR uses for RDS's 27MB file does not scale to EC2's 481MB
and needs its own design (true streaming decode, or a targeted per-SKU strategy), not
a one-line addition to `pricingfetch.Fetch`.
