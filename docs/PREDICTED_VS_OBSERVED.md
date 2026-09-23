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

## What this adds up to

Two real, consequential misses are documented above, both eventually caught, both with
actual engine fixes rather than caveats — and both belong to the same failure class:
independent code paths silently disagreeing about what a shared value means. That
pattern, once named, is now something this codebase tests for directly rather than
hoping not to repeat. The scenario that *didn't* miss (zone loss) is included for the
same reason the misses are: a report that never shows its work failing isn't
trustworthy about the parts where it succeeded either.
