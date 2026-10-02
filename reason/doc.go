// Package reason is the LLM annotation layer (P2, ADR-005): direct net/http against
// NVIDIA's OpenAI-compatible chat-completions API, no SDK of any kind. Per invariant
// I3 it receives a frozen, already-returned set of findings and produces SEPARATE
// annotations, provenance-tagged llm_reasoned and citing the evidence they reasoned
// from. It never mutates the deterministic result, and nothing in the assessment path
// calls it (NFR-11: the LLM is optional; when it is down the product degrades).
//
// The stream shape it parses was verified against a live response on 2026-10-02
// (testdata/nvidia_stream_real.sse is that capture): OpenAI-style
// `data: {...choices[0].delta.content...}` lines, `data: [DONE]` at the end, a final
// usage chunk whose `choices` is EMPTY, and — for the reasoning model chosen in
// ADR-005 — `delta.reasoning_content` chunks BEFORE the answer. Reasoning is never
// part of the narrative, and it consumes max_tokens, so callers must leave headroom.
package reason
