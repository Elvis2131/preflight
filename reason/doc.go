// Package reason is the LLM annotation layer (P2, ADR-005): direct net/http against
// NVIDIA's OpenAI-compatible chat-completions API, no SDK. Per invariant I3, it
// receives a frozen, already-returned Assessment and produces a separate annotation
// merged additively and provenance-tagged llm_reasoned — it never mutates the
// deterministic result. PC-77 builds the real client.
package reason
