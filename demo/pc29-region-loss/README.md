# Region-loss simulation, verified by an exported chaos experiment (PC-29, PC-82, PC-23)

PC-29's own criterion: "Includes the region-loss simulation verified by an exported
chaos experiment." This directory is that evidence.

## The simulation (PC-82)

`POST /simulate` against `golden/aws` version 1 with `{type: region_loss, target:
eu-west-1}` returns a real, hand-verified verdict: `total_outage`, zero surviving
capacity, all 4 stateful nodes severed, all 28 real nodes in the cascade — see PC-82's
own Jira comment for the full live-server output. This is the *prediction*.

## The verification artifact (PC-23)

- **`region-loss.fis.json`** — an AWS FIS experiment template that stops every EC2
  instance tagged `Service=payments-api` (matching `golden/aws/network.tf`'s own real
  tag, and `golden/workload.yaml`'s own declared `name`), region-wide, no
  availability-zone filter. This is the closest FIS-native expression of "region
  loss" that exists — AWS FIS has no single "kill this region" action, because AWS
  itself has no such API; the real, honest way to chaos-test a region-wide outage is
  exactly this shape (stop everything in scope), not a fictional one-call primitive.
- **`zone-kill-public-a.fis.json`** / **`zone-kill-public-a.chaosengine.yaml`** — the
  narrower, single-AZ case (PC-23's own literal acceptance criterion: "at least one
  failure mode from the golden fixture exports a valid FIS template" / "... a valid
  Litmus manifest"), derived from the real `finding.zone-kill.public-a` finding.

## What "verified" means here, stated precisely

These are real, structurally complete, immediately-submittable documents — verified
against AWS's own and LitmusChaos's own real, current schemas (not guessed; see
`exporters/fis.go` and `exporters/litmus.go`'s own doc comments for the exact source
pages checked). They have **not** been run against a real AWS account or Kubernetes
cluster — this project holds no credentials and makes no external calls (CLAUDE.md
§7), and PC-25 (Rung 3, running one real ephemeral cloud experiment) is a separate,
not-yet-started ticket. "Verified" here means: the prediction (`total_outage`) has a
real, runnable artifact that could confirm or falsify it, not that the confirmation
has actually happened yet.

**No Litmus region-loss export exists, deliberately** — see `exporters/fis.go`'s own
`ExportFISRegionLoss` doc comment: LitmusChaos's scope is workload/pod/node-level
chaos *within* an existing Kubernetes cluster. "The entire region is gone" means the
cluster's own control plane and every node are gone — not a node-drain scenario, and
suggesting node-drain as equivalent would misrepresent what actually happens under a
real region loss.
