# Predicted vs. observed: where the model was wrong, and what changed

*PC-26. A companion to [the IR design note](IR_DESIGN_NOTE.md) — that note explains
the choices; this one is the accountability check on them.*

Every claim in this document is checked against this repo's own real commit history,
test suite, and Jira record — not reconstructed from memory. Three of the six golden
failure scenarios are covered below. One of them includes a real miss: a wrong
prediction, caught in a live run rather than a unit test, with an actual engine fix,
not a caveat bolted onto the output.

**A scoping note, stated rather than implied**: "observed" here means *independently
verified ground truth* — hand-computation against the real Terraform, or direct
verification against primary AWS/Azure documentation — not a live cloud experiment.
Running a real, ephemeral chaos experiment against a deployed account (Rung 3 of this
project's own validation ladder) is separate, not-yet-started work (PC-25). This
document is honest about that boundary rather than implying more empirical weight than
these predictions currently carry.

## Scenario 1: availability-zone loss

**Prediction**: killing `eu-west-1a`'s public subnet in the golden AWS architecture
should sever the load balancer and the single NAT gateway serving all three subnets —
a real, non-trivial blast radius, not a vacuous empty set.

**Verification**: computed directly against the real parsed IR (`core.
ContainmentBlastRadius`, a pure graph reachability function), then hand-counted
against `golden/aws/network.tf` by reading the actual resource graph line by line.
Matched exactly: 2 components affected (`aws_lb.payments`, `aws_nat_gateway.nat_a`).

**Outcome**: correct on the first real run. No miss to report here — included because
a predicted-vs-observed writeup that only reports the score at 100% for every scenario
would itself be a form of the "looks confident, isn't checked" failure this whole
project exists to catch. This scenario earned its confidence; the next one didn't at
first.

## Scenario 2: database failure — the real miss

**Prediction (early version)**: given a database's `multi_az`/`zone_redundant`
setting and a declared RPO target, the tool predicts whether that target is
structurally feasible. A database with **no** synchronous standby (`multi_az=false`)
should be reported as having **no** automatic failover path — `RTOFeasibility` should
read `Assessed(false)`.

**What actually happened**: it read `Assessed(true)`. The failover-mechanism producer
function returned a full descriptive sentence for the "no standby" case — a string
*starting with* "none" (`"none — no standby exists..."`) — while the consumer function
checked for an *exact* match against the bare string `"none"`. The two never matched,
so the "no failover exists" case fell through to "a failover mechanism was
specified" → `true`. **A database with zero redundancy would have been reported as
resilient** — the most consequential kind of wrong answer this tool can give, silently
inverting the actual risk.

**How it was caught**: not by a unit test written with matched, hand-picked fixtures —
those would have used internally-consistent producer/consumer strings and passed
cleanly. It surfaced against the real golden-broken bundle, where the producer and
consumer were exercised by genuinely independent code paths for the first time.

**The fix**: a single shared sentinel constant (`FailoverMechanismNone`), used by both
the producer and the consumer, so the two code paths can no longer drift apart
independently. Not a caveat added to the output — the underlying contract between the
two functions changed.

**Second instance of the same failure class, later, at a different layer**: the
scorecard/delta engine had an equivalent bug. A finding that was `not_assessable` in
one version and became `satisfied` in the next (a database's encryption and
replication both fixed at once) was classified as a plain `improvement` — the same
label used when an already-*known* bad state gets better. That's not the same claim:
"an unknown resolved into a known-good state" and "a known-bad state got better" are
different epistemic events, and collapsing them loses exactly the distinction a
reviewer would want ("did we already know this was a risk, or did we just find out it
wasn't one?").

This one was caught live — not in a hand-written test, but while driving an actual
agent-iteration demo end to end against a real running server, reading the real
`assurance_delta` field in the response and noticing the label didn't match the
transition. The fix added a sixth classification (`resolved_risk`), distinct from
`improvement`, applied consistently, and was then verified on **two independent
clouds** (AWS and Azure) to confirm it wasn't a fixture-specific patch.

**The pattern connecting both**: whenever two independent code paths need to agree on
a sentinel value or a classification boundary, unit tests built from matched,
hand-picked fixtures will not catch a drift between them — only a real, end-to-end run
exercising both paths together will. Both fixes now have a permanent regression test
built specifically to catch a reintroduction of the same class of bug, not just the
specific instance.

## Scenario 3: region loss

**Prediction**: the golden architecture declares exactly one AWS region with no
cross-region failover for anything. A `region_loss` fault should therefore predict a
**total outage** — zero surviving entry points, zero surviving capacity, every
stateful resource severed — not a partial degradation.

**Verification, in two independent forms**:
1. Computed directly (`core.Simulate`, reusing the same forward-reachability and
   declared-capacity engines the zone-loss scenario above uses — not a second,
   parallel implementation), then hand-verified against the real golden IR: all 4
   stateful resources severed, all 28 real nodes in the cascade, zero surviving
   `app_node_rps` capacity.
2. Exported as a real, structurally valid AWS FIS experiment template — a document
   that, given a real AWS account and role, would actually execute the predicted
   fault (stop every instance belonging to the workload, region-wide) rather than
   remaining a claim on paper. See `demo/pc29-region-loss/` for the generated
   artifact.

**Outcome**: correct, and — unlike the other two scenarios — backed by a document that
could, if handed to someone with a real AWS account, actually go test it. That's the
difference this project's own thesis rests on: not just predicting what fails, but
handing over something that can prove the prediction right or wrong.

## Scenario 4: degraded-state latency (PC-128, Layer 3) — a real miss, and a measurement that was wrong first

**Prediction**: `core.ComputeDegradedLatency` models each component as an M/M/1 station
(Poisson arrivals, exponential service, service rate = the declared capacity), so a
component's mean sojourn time is `1/(mu - lambda)` and its p95 is `mean x ln 20`.

**Verification (Rung 1 — a local replica, not cloud-native semantics or real capacity)**:
a real PostgreSQL 16 server (`postgres:16-alpine`), driven by one `pgbench` client — one
server with a queue in front — issuing open-loop Poisson arrivals (`-R`) at utilisations
0.3 to 0.9, three runs per level, with the per-transaction latency log. The service rate is
the only thing taken from the measurement, and it is exactly what a user *declares*
(capacity), so the comparison is not circular: everything else is the engine's prediction
against what a server we did not write actually did. Harness:
`core/latency_rung1_test.go` (`go test -tags live_rung1 ./core/ -run Rung1_Latency`).

**First, the measurement was wrong, and I only caught it because the numbers were too
good to be queueing.** An early version of the harness added pgbench's `schedule lag` to its
`time` column. Reading pgbench's own summary showed `latency average` equals the mean of the
`time` column: it already includes the lag. Every observed latency in that version was
inflated, and those results were discarded (not recorded). The two means-only runs that used
pgbench's own summary were unaffected and are kept.

**Outcome 1 — the sub-millisecond select (`SELECT` at ~0.07 ms, calibrated `mu` ~13.4k tps,
service-time SCV 6.4): a large miss with a cause I can only partly attribute.** Observed mean
latency was 1.0 to 1.1 ms against a predicted 0.11 to 0.15 ms at utilisations 0.3 to 0.5 (~7-10x),
and rare stalls (p99 13-625 ms, one 84 ms service-time outlier in calibration) dominated the
mean at higher load. Even the *median* stayed ~0.6-0.7 ms at every utilisation from 0.3 to 0.8:
latency that does not move with load is not queueing, it is a floor from the load
generator's ~1 ms timer wake-up in the Docker VM. So this regime measures the apparatus as much
as the model, and I do not claim it validates or refutes M/M/1. What it does show, and the
model should say, is that a declared capacity taken from a closed-loop saturation
benchmark is not a sustainable open-loop capacity: at nominal utilisation 0.9 the system was
effectively saturated (hundreds of ms in one run) while the model predicted 0.75 ms.

**Outcome 2 — a 10 ms service (`SELECT pg_sleep(0.010)`, measured service time ~13.3 ms,
SCV 0.02-0.03, i.e. near-deterministic): a clean, understood miss.** The apparatus noise
(~1 ms) is small next to the service time here. Mean latency, ms (two clean runs):

| utilisation | M/M/1 (engine, before) | M/G/1 with measured SCV | observed run 1 | observed run 2 |
|---|---|---|---|---|
| 0.3 | 18.9 / 19.1 | 16.1 / 16.3 | 20.6 | 20.0 |
| 0.5 | 26.4 / 26.8 | 19.9 / 20.2 | 23.9 | 23.0 |
| 0.7 | 44.1 / 44.6 | 28.8 / 29.3 | 30.8 | 33.1 |
| 0.8 | 66.1 / 66.9 | 40.0 / 40.5 | 42.9 | 39.2 |
| 0.9 | 132.2 / 133.8 | 73.5 / 73.9 | 79.8 | 53.1 |

M/M/1 **overestimated** the mean by 30-60% from 0.7 upward, and its p95 (`mean x ln 20`) by
about 2x (utilisation 0.8: predicted ~200 ms, observed 94-107 ms). That is what queueing theory
says about a near-deterministic server: waiting is roughly half the exponential case. The
Pollaczek-Khinchine M/G/1 mean using the measured SCV is within about 10% from 0.5 to 0.8 and
tracks the trend at 0.9, where run-to-run spread is large (53 vs 80 ms observed against 74 ms
predicted): with 15-30 s runs, high-utilisation means carry roughly +/-30% noise, so I do not
claim better than that. At low load observed latency sits a few ms above both models (the
pacing floor again).

**What changed (an engine fix, not only a caveat):** `Workload.ServiceTimeSCV` (workload schema
1.6.0, additive) lets the architect declare a component type's service-time variability, keyed
like capacity. Declared: the mean uses the Pollaczek-Khinchine formula and the p95 is withheld
(its closed form needs exponential sojourn times). Not declared: exactly the old M/M/1 output,
now with the assumption stated and the Rung 1 finding cited in the result's assumptions. SCV = 1
reproduces M/M/1 exactly and SCV = 0 is M/D/1 (both hand-verified in tests), and a test feeds the
measured calibration from this experiment's recorded evidence through the engine and requires
it to reproduce the recorded M/G/1 numbers. The model is still never given a service time or an
SCV it was not declared.

**Limits, stated rather than implied:** one service type on one laptop VM; three short runs per
level; the sub-millisecond regime is not resolvable with this harness; the model is
single-station per component and does not model concurrency, so this says nothing about a real
multi-core database. Evidence: `docs/eval/latency-predicted-vs-observed-10ms-service.json`
(+ `-run1`), `latency-predicted-vs-observed-fast-select.json`, and the two earlier means-only
fast-select runs `latency-fast-select-means-only-1/2.json`.

## Scenario 5: a seeded IAM defect the engine never saw (PC-157) — a real miss

**Predicted:** golden/aws-broken seeds defect 7, an application role with `Action = "*"` on
`Resource = "*"`, so the engine's least-privilege check should report that role `unsatisfied`, and
golden/aws's scoped role should report `satisfied`.

**Observed:** the broken role reported **satisfied**, and so did the good one. Neither was ever
evaluated. Both bundles write the policy as `jsonencode({...})`; ingest only read literal strings, so
the document was silently dropped, the role ended up with no statements, and the evaluator read "no
statement allows it" as an implicit deny. The agent-iteration acceptance test passed throughout because
it never needed to fix defect 7.

**How it surfaced:** reading the SOC 2 and PCI DSS criteria to cite CC6.3 and 7.2.2 against the existing
IAM evidence (PC-121). A control that cites "least privilege" has to be checked against a bundle that
violates it, and this one did not fail.

**Class:** the same one as the misses above, an input silently dropped between two components that each
believed the other handled it, plus an I4 failure (an unread document is not a pass).

**Fix:** ingest evaluates `jsonencode` of constants, so the policy is read; a policy it still cannot read
(it references a resource or variable, is malformed, or is an AWS-managed policy defined outside the
bundle) is recorded on the role and makes every IAM evaluation of it `not_assessable`, because an unread
statement could hold an explicit Deny. Negative controls: dropping either half fails tests. Result:
defect 7 is now `unsatisfied`; golden/aws's roles are honestly `not_assessable` (their policies reference
resource ARNs, and two attach AWS-managed policies), where they used to read as a vacuous `satisfied`. The
agent-iteration loop now fixes defect 7 too and still converges in 2 iterations (bound 5).

**Limit, stated:** a policy that references a resource ARN is not assessable at all today. Evaluating the
parts of such a policy that do not depend on the unresolved ARN is a larger change (tri-state resource
matching) and is not attempted here.

## What this adds up to

Several real, consequential misses are documented above (the latency-model one's measurement was itself wrong first), both eventually caught, both with
actual engine fixes rather than caveats — and both belong to the same failure class:
independent code paths silently disagreeing about what a shared value means. That
pattern, once named, is now something this codebase tests for directly rather than
hoping not to repeat. The scenario that *didn't* miss (zone loss) is included for the
same reason the misses are: a report that never shows its work failing isn't
trustworthy about the parts where it succeeded either.
