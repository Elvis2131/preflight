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

## Not decided here

**The specific model is deliberately not chosen.** Per PC-77: select it by
narrative-quality evaluation against the golden fixture's findings, once `ingest/` and
`analyse/` produce real findings to narrate. Choosing a model before there is anything
to narrate would be picking on vibes.

When chosen, record the model **and the eval it was chosen against** — the latter is the
acceptance criterion, not the former.

## Revisit trigger

Reopen if any of the following becomes true:

1. The free tier's rate limit begins degrading the narrative often enough to be a product
   complaint rather than an occasional gap.
2. `reason/` is ever proposed to do anything beyond annotation — this would violate I3 and
   invalidates the entire "low-stakes provider" argument.
3. The OpenAI-compatible shape stops being a reliable common denominator.
