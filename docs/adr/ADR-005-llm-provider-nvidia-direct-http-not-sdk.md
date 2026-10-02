# ADR-005: LLM provider for `reason/` — NVIDIA API, direct HTTP, no SDK

- **Status:** Accepted
- **Date decided:** not recorded in the available source
- **Ratified into this log:** 2026-09-20 (PC-77)
- **Provenance:** reconstituted from the PC-77 issue record, with supporting context from
  PC-8 and PC-21. No pre-existing ADR-005 document was available.

## Context

`reason/` is the LLM layer (process P2, ADR-003). Under invariant I3 (CLAUDE.md §4) it
**annotates, never mutates**: it receives a frozen, already-returned assessment and its
output is merged additively, tagged `llm_reasoned`. It cannot affect a verdict.

That constraint is what makes this decision low-stakes in a way it would not otherwise
be — and it is the whole reason a free, uncontracted provider is a reasonable choice
here rather than a reckless one.

## Decision

Two parts:

1. **Provider: NVIDIA's hosted API**, on its free tier.
2. **No SDK at all** — direct `net/http` against the OpenAI-compatible chat-completions
   shape.

"No SDK" means no SDK: not Anthropic's, and not an OpenAI-compatible Go client either.
This is consistent with the existing `net/http` + `chi` choice for the API layer rather
than a special case.

## Rationale

- **I3 caps the blast radius.** A narrative-only call that can never change a verdict is
  the one place in this system where a lower-guarantee provider is acceptable.
- **NFR-11 already covers the risk.** "LLM optional, deterministic path never blocked"
  was written *before* this decision, but it is exactly what makes NVIDIA's uncontracted
  ~40 RPM free tier tolerable. Rate limiting or outright unavailability degrades the
  narrative; it cannot block an assessment. PC-21 makes this testable: killing the LLM
  connection mid-request must still return a complete deterministic payload with a
  degraded flag set.
- **An SDK buys nothing here.** One chat-completions call against a documented JSON
  shape does not justify a dependency, and avoiding one keeps provider substitution a
  matter of changing a URL and a request struct.

## Rejected alternatives

- **Anthropic API via `anthropic-sdk-go`.** The prior assumption, cited in PC-8 as
  evidence of Go ecosystem parity. Superseded here. PC-8's underlying point about Go
  ecosystem parity stands; this specific example no longer does.
- **An OpenAI-compatible Go client library.** Rejected for the same reason as any other
  SDK: a dependency for a single documented HTTP call.

## Consequences

- `reason/`'s HTTP client must have **zero** SDK dependencies (PC-77 acceptance criteria).
- **Streaming shapes differ and this must be confirmed before SSE handling is built, not
  discovered afterwards.** OpenAI-compatible streaming delivers
  `choices[0].delta.content`; Anthropic's delivers `content_block_delta`. PC-21 plans SSE
  for the narrative stream, so this is load-bearing.
- CLAUDE.md §2 and §7 predate this ADR and still describe an Anthropic SDK and "Claude API
  calls" in P2. Both need correcting to match this decision.

## Verified 2026-10-02 (PC-77)

Against a live response from `https://integrate.api.nvidia.com/v1/chat/completions`
(capture: `reason/testdata/nvidia_stream_real.sse`):

- The streaming shape is OpenAI-style — `data: {...}` lines whose answer text is
  `choices[0].delta.content`, ending with `data: [DONE]` — and is **not** Anthropic's
  `content_block_delta`. Confirmed before SSE handling was built.
- **The reasoning model streams `choices[0].delta.reasoning_content` before `content`.**
  Reasoning is scratch work and is never part of a narrative; the client counts it and
  discards it.
- **The final chunk has an empty `choices` array** and carries `usage`.
- **Reasoning tokens are billed against `max_tokens`** (38 of 53 completion tokens in the
  probe). A small cap can leave `content` empty; the client reports that as an error and
  the annotator uses a generous budget.
- `reason/` imports only the standard library and `preflight/core`; `go.mod` contains no
  LLM SDK. Both are enforced by tests (`reason/nosdk_test.go`), not by convention.

## Not decided here

**The model.** `nvidia/nemotron-3-super-120b-a12b` was named by the maintainer on PC-77
(2026-10-02) and is the model `reason/` is configured and tested against. The acceptance
criterion is to record the eval it was chosen against; the eval is `cmd/reason-eval`,
which scores one model on the golden findings with checks that do not trust the model:
coverage, real citations, no fabricated likelihood for a `not_assessable` dimension,
resistance to a planted prompt injection, and findings unchanged. **Status: the harness
exists and is tested offline; the recorded run is pending** (it needs `NVIDIA_API_KEY`).
It is an acceptance eval of one model, not a comparison between models, and it does not
judge prose quality, which needs a human reader.

## Revisit trigger

Reopen if any of the following becomes true:

1. The free tier's rate limit begins degrading the narrative often enough to be a product
   complaint rather than an occasional gap.
2. `reason/` is ever proposed to do anything beyond annotation — this would violate I3 and
   invalidates the entire "low-stakes provider" argument.
3. The OpenAI-compatible shape stops being a reliable common denominator.
