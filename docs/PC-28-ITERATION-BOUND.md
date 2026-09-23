# PC-28: the iteration bound, decided before the harness was written

- **Status:** Decided
- **Date:** 2026-09-21
- **Scope:** the CI-deterministic stubbed-agent acceptance test only (PC-28's own
  scope — a separate live-agent demo run is PC-29's job, not this one's).

## Why this needs deciding up front

The harness drives `golden/aws-broken` through repeated `server.Assess` calls, applying
a scripted fix each round, until the findings that matter converge to a passing state.
Something has to bound that loop. Picking the bound empirically — running the loop with
no cap, seeing how many rounds it actually took, and hard-coding that number after the
fact — would silently make the test unable to ever report "failed to converge": it would
have been tuned to whatever the implementation happened to do, not derived from an
independent expectation of how many rounds a real fix-and-recheck cycle should
reasonably take. That defeats the point of the assertion. The bound must come from the
problem shape, decided before the loop is written, exactly as the min-cut algorithm
choice (PC-14) was picked from the problem shape before it was tested against real data.

## The problem shape

Two golden defects are being fixed, each independently:

1. **Defect 1** (`network.tf`): single NAT gateway serving three AZs. Fix: add
   `nat_b`/`nat_c` (EIP + gateway) and repoint `private_b`/`private_c`'s route tables.
   Detected by `finding.compliance.nat-gateway-redundancy` (PC-28's own new finding).
2. **Defect 2** (`rds.tf`): `aws_db_instance.payments` has `multi_az = false` and
   `storage_encrypted = false`. Fix: flip both to `true`. Detected by
   `finding.compliance.rds-storage-encryption...` (PC-18) and
   `finding.compliance.rds-rpo-feasibility...` (PC-28's own other new finding).

Each defect has exactly one scripted fix, and the two fixes are independent edits to
different files — nothing about applying one changes whether the other is still needed.
A correctly-scripted agent converges in exactly 2 rounds: one Terraform edit per round,
each followed by a re-`Assess` that confirms that defect's finding(s) flipped.

## The decision: N = 5

**5 iterations.** Not 2 (the exact number of fixes), and not open-ended.

- **Margin over the minimum, not equal to it:** 2 is the happy-path count with zero
  slack. A real agent (or a scripted stand-in standing for one) may re-check a fix that
  didn't fully land, or apply a fix that only partially addresses a finding's stated
  rationale before getting it right — PC-28's Card explicitly frames iteration as the
  point being exercised, not a formality. 5 gives 3 rounds of margin over the 2 needed,
  without being so generous that a genuinely broken convergence (the fix logic doesn't
  work, or a finding never flips) is masked by simply running it more times.
- **Not open-ended:** an uncapped loop can't distinguish "still working" from "will
  never converge," and a CI test must terminate deterministically either way. Failing
  at iteration 6 with a clear "did not converge within N=5 iterations, unsatisfied
  findings: [...]" assertion is itself a useful, actionable test failure — an infinite
  or very-large bound would just make that failure slower and vaguer.
- **Small enough to fail fast in CI:** each iteration is one real `ingest` + `BuildFindings`
  pass (sub-second per the golden bundle's own size) — 5 rounds costs nothing operationally,
  so the bound is set by the reasoning above, not by a runtime budget.

## What this is not

This bound is specific to this harness, against this fixed set of two known defects. It
is not a general claim about how many iterations any future architecture or defect set
should take to converge — a different bundle with more or less independent defects would
warrant a different, similarly-derived bound, not a reuse of this one by default.
