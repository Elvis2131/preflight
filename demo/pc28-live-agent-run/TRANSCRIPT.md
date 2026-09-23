# Live-agent run: PC-28 criterion 3 / PC-29's first prong

This is a **live agent's real run** against a real, running `assessd` — not the
deterministic CI harness (`server/agent_iteration_acceptance_test.go`), and not a
replay. The agent (this session, acting as an engineer) called the real `/assess` HTTP
endpoint, read each response's actual findings, and decided what to fix from that
evidence — the same author → evaluate → modify → re-evaluate loop CLAUDE.md §3
describes, exercised for real rather than pre-scripted.

Fulfils **PC-28's own acceptance criterion 3** ("a separate live-agent run is recorded
for the demo, feeds PC-29") — recorded here because that criterion was left open when
PC-28 was closed. Also serves as **PC-29's first prong** (the recording captured now,
per the "capture now, narrate later" decision — `reason/` does not exist yet, ADR-005/
PC-77, so this run has no narrative channel; PC-21's own acceptance criterion — "killing
the LLM connection mid-request still returns a complete deterministic payload with a
degraded flag set" — is exactly the property that makes this valid evidence today rather
than something waiting on PC-77).

## Setup

- Server: `assessd` (P1), built from this repo, `PREFLIGHT_DB_PATH=/tmp/preflight-run.db`,
  listening on `:8099`.
- Bundle: a fresh working copy of `golden/aws-broken` at `demo/pc28-live-agent-run/bundle/`
  (checked-in fixture never touched — same discipline as the CI harness).
- Session: `pc29-live-agent-demo` (a session distinct from the CI harness's own
  `pc-28-acceptance`, so the two runs' persisted versions never collide in the store).
- Raw request/response JSON for every call is saved under `responses/v1.json`,
  `v2.json`, `v3.json` in this directory — nothing summarized below is invented, all of
  it traces to those files.

## Run

### Version 1 — initial assessment

`POST /assess` against the untouched broken bundle. Two findings come back
`unsatisfied`:

| Finding | Evidence |
|---|---|
| `finding.compliance.nat-gateway-redundancy` | "subnet(s) [aws_subnet.public_b aws_subnet.public_c] have no NAT gateway of their own — losing their AZ's gateway (or the single shared one) leaves them with no egress path" |
| `finding.compliance.rds-storage-encryption.aws_db_instance.payments` | "storage_encrypted is false — database files, snapshots, and automated backups remain in plaintext..." |

A third finding, `finding.compliance.rds-rpo-feasibility.aws_db_instance.payments`, is
`not_assessable` ("replication mode \"none\" has no known RPO feasibility rule") — not
flagged as failing, but worth reading rather than ignoring.

**Agent's decision:** fix the RDS resource first. Reading the whole
`aws_db_instance.payments` block (not just the one flagged attribute) surfaces that
`multi_az = false` sits right next to `storage_encrypted = false` — and the workload
(`golden/workload.yaml`) declares `rpo_seconds: 0` as a hard requirement. Even though the
RPO finding itself reads `not_assessable` rather than `unsatisfied` (the engine
deliberately declines to assert infeasibility for a replication mode it has no defined
rule for, rather than guess), a database with no standby cannot honestly satisfy an RPO
of zero. Both `storage_encrypted` and `multi_az` are flipped to `true` in one edit.

### Version 2 — after the RDS fix

`POST /assess` again, same session. Real `assurance_delta` comes back:

```
finding.compliance.rds-storage-encryption...: improvement (unsatisfied -> satisfied)
finding.compliance.rds-rpo-feasibility...:     improvement (-> satisfied)
finding.compliance.nat-gateway-redundancy:      unchanged  (unsatisfied -> unsatisfied)
```

RDS is fully resolved. NAT redundancy is untouched, as expected — that fix hasn't been
applied yet.

**Note, not silently passed over:** the RPO finding's delta reads `improvement`, not
`not_assessable` — even though it went from an *unknown* state to a *known-good* one.
This is a real, previously-undocumented discrepancy in `core.BuildScorecard`
(it excludes `not_assessable` findings from the scorecard map entirely, so
`ComputeDelta` sees this as a brand-new finding rather than "present in both, one side
unrankable" — see the open discussion on this thread). Left as-is here, not patched
mid-recording. **Root-caused and fixed under PC-83 — see the Addendum below**, which
also corrects the record: the right classification turned out to be neither
`improvement` nor `not_assessable`, but a new, distinct `resolved_risk` kind.

**Agent's decision:** fix NAT redundancy — add a NAT gateway per remaining public
subnet (`nat_b`, `nat_c`) and repoint the `private_b`/`private_c` route tables to their
own AZ's gateway, matching the pattern the clean `golden/aws` bundle already uses.

### Version 3 — after the NAT fix

`POST /assess` again. All three compliance findings now read `satisfied`:

```
finding.compliance.nat-gateway-redundancy:      improvement (unsatisfied -> satisfied)
finding.compliance.rds-storage-encryption...:   unchanged   (satisfied -> satisfied)
finding.compliance.rds-rpo-feasibility...:      unchanged   (satisfied -> satisfied)
```

Zero unsatisfied findings remain. **Converged in 2 real fix-rounds** — matching exactly
what `docs/PC-28-ITERATION-BOUND.md` predicted from the problem shape (2 independent
defects, 2 minimum rounds) before either the CI harness or this live run existed.

`terraform validate` against the fixed working copy passes (real check, not skipped —
see `bundle/.terraform.lock.hcl`, carried over from `golden/aws-broken`'s own
already-initialized provider cache).

## Addendum: PC-83 fix verified against this exact comparison

The discrepancy flagged above (`rds-rpo-feasibility`'s `not_assessable → satisfied`
transition reading `improvement`) was root-caused and fixed under PC-83:
`core.BuildScorecard` was silently excluding `not_assessable` findings from the
scorecard map entirely, so `core.ComputeDelta` saw a finding that existed the whole
time as brand-new instead of "present in both, one side unrankable." Fixed by
including every finding in the scorecard (unassessed ones get the literal status
`"not_assessable"`), and adding a new `DeltaKind` — `resolved_risk` — distinct from
`improvement` for exactly this case (PRD §5.7's own `resolved_risks[]` category,
previously unimplemented).

Re-ran this exact comparison (fresh copy of `golden/aws-broken` vs this directory's
fixed `bundle/`) against the rebuilt server. Confirmed corrected:

```
finding.compliance.nat-gateway-redundancy:      improvement    (unsatisfied -> satisfied)
finding.compliance.rds-rpo-feasibility...:       resolved_risk  (not_assessable -> satisfied)   <- was "improvement"
finding.compliance.rds-storage-encryption...:    improvement    (unsatisfied -> satisfied)
finding.zone-kill.data-a:                        not_assessable (not_assessable -> not_assessable) <- was silently absent from the delta entirely
finding.zone-kill.public-a:                      not_assessable (2 component(s) affected -> 2 component(s) affected)
```

The contradiction no longer reproduces. This transcript's evidence — 2-round
convergence, real Terraform edits, matching the predicted iteration bound — was already
valid independent of this bug and was not re-run; only the delta classification above
needed re-verification, per PC-83's own acceptance criterion.

## What this is, and isn't

- **Is:** a real, reproducible live-agent run against the real HTTP API, with an agent
  making its own judgment calls from the evidence text (the multi_az call above wasn't
  spelled out by any single finding — it came from reading the resource, the way an
  engineer would). Diffs applied: `bundle/rds.tf`, `bundle/network.tf` (both included in
  this directory for inspection).
- **Isn't:** a polished, narrated recording for actual publication — this run has no
  video/audio, and `reason/`'s narrative channel doesn't exist yet (ADR-005/PC-77). Where
  and how this gets published (Reppl.sh, format, whether it's re-recorded with narration
  once `reason/` lands) is PC-29's own open question, not decided here.
- **Isn't** a substitute for PC-29's other two prongs (Azure via PC-22, region-loss via
  PC-82+PC-23) — this covers PC-29's AWS-side "one full iteration cycle" criterion only.
