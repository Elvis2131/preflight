# NFR-9 first-annotation latency, measured live (PC-154)

**NFR-9** (technical design §4): LLM narrative first token < 3 s after the deterministic payload.
**Result: NOT MET.** Measured against the real model, first-annotation latency was 6.4 – 10.5 s.

## Method
- 2026-10-02. `reasond` (P2, holds the key) and `assessd` (P1, no key, `PREFLIGHT_REASOND_URL`) built from the
  repo at 5b9755c plus the PC-152 commit, on throwaway SQLite state. Model `nvidia/nemotron-3-super-120b-a12b`.
- Golden fixture (`golden/aws`, `golden/workload.yaml`), 18 findings. For each run: a fresh session, `POST /assess`,
  then `GET /sessions/{id}/versions/1/annotations` (SSE). Time is measured from the start of the annotations
  request (i.e. after the deterministic payload was returned) to the first `event: annotation`.
- Each run was read to `event: done` before the next began, so every run starts against an idle worker.
- Client script: not committed (about 30 lines of Python over `urllib`); the numbers below are its raw output.

## Numbers
| run | /assess (NFR-6, < 5 s) | first annotation (NFR-9, < 3 s) |
|---|---|---|
| 1 | 0.266 s | 10.54 s |
| 2 | 0.247 s | 6.79 s |
| 3 | 0.191 s | 6.41 s |

NFR-6 holds with a wide margin. NFR-9 misses by a factor of roughly 2 – 3.5.
Three runs is a small sample: it shows the miss, not a distribution.

## Reading
The model reasons before it answers (about 10 s per finding in the PC-77 eval). The first annotation therefore
cannot arrive in 3 s however fast the plumbing is; the plumbing was already shown to be well inside 3 s with an
instant worker (`server/annotations_test.go`). The 3 s target was set before a reasoning model was chosen.
**Proposal (needs a decision, not made here):** revisit NFR-9 explicitly — either restate it as time to the first
*stream event* (`started`) with the first annotation as a separate, model-bound figure, or choose a non-reasoning
model/setting, or accept a looser target with a recorded reason. The test and the target are unchanged.

## An earlier attempt, recorded because it was misleading
A first harness closed the stream after the first annotation. Runs 2 – 4 then took 182 – 204 s to their first
annotation (run 1 was 10.15 s). The probable cause is that the abandoned run kept the serialised worker busy, but
that was inferred, not verified; those numbers are not used above. If it holds, a client disconnecting mid-stream
does not free the worker, which is worth its own look.

## Detection overstatement (PC-154 extra criterion)
Fresh live eval, same model: 18/18 accepted, 0 rejected, fabricated-likelihood 0, **detection-overstated 0**,
injection complied = false, findings unchanged = true. `go run ./cmd/reason-eval -check-detection
docs/eval/reason-eval.json` → 0 of 18. The previous report is kept as `reason-eval-pc77-run1.json`
(it scored 1 of 17).

## Sweep by reasoning mode (PC-160)
Same method, fresh session per run, each run drained to `done`. Raw output: `nfr9-latency-sweep-raw.txt`.
Modes use NVIDIA's documented `chat_template_kwargs` (`enable_thinking`, `low_effort`; model card), opt-in via
`PREFLIGHT_REASON_REASONING` on `reasond`. Default behaviour is unchanged.

| mode | runs | median | min | max | runs under 3 s | sorted first-annotation seconds |
|---|---|---|---|---|---|---|
| provider default | 6 (3 + 3, see note) | ~9 s | 6.4 | 46.5 | 0 of 6 | 6.41, 6.79, 7.59, 10.54, 38.9, 46.45 |
| `off` | 10 | 3.8 s | 1.3 | 24.6 | 4 of 10 | 1.26, 1.31, 2.12, 2.23, 3.23, 4.32, 4.43, 9.47, 14.04, 24.55 |
| `low_effort` | 10 | 2.4 s | 0.9 | 5.2 | 7 of 10 | 0.94, 1.53, 1.58, 2.05, 2.16, 2.58, 2.73, 3.49, 4.21, 5.21 |

Note on the default row: the 10-run default sweep stopped after 3 runs (the sweep process was no longer
running when checked; cause not established), so the default is the 3 runs above plus the 3 from the first
measurement. Two default runs (38.9 s, 46.5 s) are far above the earlier 6.4 - 10.5 s; free-tier contention is a
plausible cause but was not verified.

Full-run eval per mode (18 findings each, same scorer; reports in this folder):

| mode | accepted | rejected | fabricated likelihood | detection overstated | injection complied | total time | tokens |
|---|---|---|---|---|---|---|---|
| default | 18/18 | 0 | 0 | 0 | no | 168 s | 32,061 |
| `off` | 18/18 | 0 | 0 | 0 | no | 55 s | 17,685 |
| `low_effort` | 18/18 | 0 | 0 | 0 | no | 50 s | 19,288 |

On a read-through of three findings side by side, the narratives in the three modes say the same things; `off` and
`low_effort` are somewhat shorter (mean 513 / 477 characters against 618).

### What this shows, and does not
- Turning reasoning down is the lever: `low_effort` gets the median under 3 s and cuts a full run from 168 s to
  50 s, with no loss on any scored check. Even so only 7 of 10 first annotations met 3 s, and the worst was 5.2 s.
  A hard "under 3 s" is therefore not reliably met in any mode on this free tier. Variance is large (`off` ranged
  1.3 - 24.6 s), so ten runs per mode is still a small sample.
- Not shown: quality beyond the automated checks and one short read-through; behaviour under a paid tier or
  concurrent users (P2 serialises runs, `reason/handler_test.go` pins it).

### Disconnect behaviour (verified)
`reasond` cancels a run when its client hangs up (a first draft of the pinning test freed the worker exactly
that way). P1 does not hang up: it runs the job detached so a complete set can be stored for replay (ADR-005).
So the 180 - 200 s waits in the first attempt were the P1 job of the abandoned request still holding the
serialised worker. That is by design; the consequence is head-of-line blocking between sessions. Pinned by
`TestHandler_SecondRunWaitsForTheFirst`.

### Options for NFR-9 (decision for the owner; none applied)
1. **Recommended: restate and tune.** Make `low_effort` the reasond default and restate NFR-9 as a percentile
   (for example first annotation within 5 s for at least 90% of runs, measured live), keeping "never blocks
   NFR-6" as is. The measured data supports roughly this and no more.
2. Restate only: first stream event (`started`) as the 3 s figure, first annotation separate. Cheapest, but the
   `started` event carries no content, so it would meet the letter and not the intent.
3. Keep 3 s and change model/hosting (paid tier, smaller non-reasoning model). Needs its own evaluation.

## Decision applied (PC-160)
`low_effort` is now the `reasond` default. 15 further runs with the default configuration and no override
(`nfr9-latency-low-effort-default-15runs.txt`): 1.08, 1.25, 1.42, 1.46, 1.88, 2.03, 3.04, 3.53, 3.58, 4.95, 5.01,
5.66, 7.07, 7.37, 7.40 s (median 3.5 s, 6 of 15 under 3 s, 10 of 15 within 5 s, all within 8 s). Combined with the
earlier 10 low-effort runs: 25 runs, median 2.7 s, p90 7.1 s, max 7.4 s, 13 of 25 under 3 s, 19 of 25 within 5 s.

My recommendation of "within 5 s for 90% of runs" did NOT survive the data (76%), so it was not adopted. NFR-9 now
reads median ≤ 4 s and p90 ≤ 8 s (technical design §3, ADR-005 amendment), a description of what this free tier
delivers, set from the measurement, with the original 3 s target's result (52% of runs) stated beside it.
