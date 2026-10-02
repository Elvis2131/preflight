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
