# Reference-architecture templates (PC-108)

Five starting points the workspace can load in one action. A template is **data**: a
checked-in canvas document, an inline workload, and a UI-only layout. Loading one puts it
in the same canvas state a hand-drawn design lives in; assessing it goes through the
ordinary `POST /sessions/{id}/canvas` path. There is no "assess template" endpoint and no
template-specific code path (`TestTemplates_AssessThroughTheOrdinaryCanvasPipeline` proves
the ordinary path returns the checked-in findings byte for byte).

| Template | What it is | Honest limits |
|---|---|---|
| `three-tier-vpc` | DNS → ALB across two AZs → app cluster → Multi-AZ database + cache, in a VPC with public/private/data subnets in two AZs, a NAT per AZ, an internet gateway, public and private route tables, an allow-all NACL, and SG rules (443 from the internet → 8080 from the LB SG → 5432 from the app SG). Three declared journeys (web, api, data) **flow end to end** through every SG, NACL and route step. | Only claims what is declared: storage encrypted, multi-AZ. The NACL is an explicit allow-all (the default-NACL behaviour made visible), not a hardened policy. |
| `serverless-api` | DNS → Lambda → DynamoDB | **No API Gateway** — it is not in the capability registry, so the template does not claim it. |
| `event-driven` | DNS → producer → SQS ← consumer → DynamoDB | Reachability follows edge direction and a consumer *pulls* from the queue, so the results table is not reachable from the entry point in either state. The failure fixture shows what the engine computes (a consumer loss leaves the producer→queue path intact), no more. |
| `simple-aws-network` | One VPC with two-AZ public/app/data subnets, private EC2, HTTPS ALB, Single-AZ private RDS, one NAT, explicit routing/SG/NACL configuration and an EC2 trust role. | App/NAT/database failure points are intentional. EC2 request simulation and local-only database routing remain unmodelled. |
| `enterprise-aws-network` | Production, non-production and shared-services VPCs; segmented TGW policy; two hybrid VPNs; hybrid Resolver DNS; private workloads; isolated Multi-AZ RDS; four AZ-local NATs; log-archive intent. | TGW/VPN/Resolver enforcement, TLS handshakes, IAM permissions and logging delivery are not assessed. Local-only data routing remains unknown. |

See [NETWORK-DESIGNS.md](NETWORK-DESIGNS.md) for the complete network designs, CIDR and
routing tables, traffic permissions, deployment choices, failure cases and AWS sources.
The new network templates preserve unmodelled design intent as explicitly named
`design_*` capability strings. This does not promote those services' capability levels.

## Fixtures

`ir.json`, `findings.json` and `failure.json` beside each template are **derived**, never
hand-written (PC-15's rule): `go run ./cmd/gen-template-fixtures`. `go test
./golden/templates` re-derives them and byte-compares. If that fails, hand-verify the change
against the template before regenerating — the files are checked-in output, not a cache.
`failure.json` is the `/simulate` result for one declared fault per template
(`failureTargets` in `derive.go`); a template without a failure-mode result does not ship.

## Known engine behaviour visible in the fixtures (not template bugs)

- `core.BuildFindings` still names golden's own subnet IDs for its two zone-kill findings and
  its NAT-coverage finding. `three-tier-vpc` uses golden-style IDs so those apply; the two
  Lambda/DynamoDB templates have no such subnets, so those three findings read
  `not_assessable` with their stated reason.
- Zone-kill counts a node contained in several subnets (the ALB) as affected by the loss of
  any one — the same containment semantics golden/aws has.
- No `rpo_seconds` requirement is declared, so RPO feasibility is `not_assessable`.

## Layout

`layout.json` is UI placement only (id → x, y, optional width/height for containers). It is
not part of `canvas.schema.json` and never an input to any verdict.
Optional `service_x` / `service_y` values provide a separate clean service-view layout.
