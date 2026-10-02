package reason

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"preflight/core"
)

// maxAnnotateBody bounds the findings payload a caller may post (a full golden assessment is
// well under 1 MB).
const maxAnnotateBody = 8 << 20

// AnnotateRequest is what P1 hands P2: the frozen, already-returned findings (I3). P2 never
// sees the IR, the workload or any credential — only what the architect already received.
type AnnotateRequest struct {
	Findings []core.Finding `json:"findings"`
}

// NewAnnotateHandler serves POST /v1/annotate for the reason worker (P2). client is nil when
// no API key is configured: the handler then answers 503 {"degraded":"no_api_key"} — a clean,
// machine-readable "narrative layer unavailable", never a crash (NFR-11). Otherwise it
// streams Server-Sent Events as each finding is annotated:
//
//	event: started    data: {"model": "...", "findings": N}   (sent immediately)
//	event: annotation data: core.LLMAnnotation
//	event: rejected   data: core.LLMRejection
//	event: done       data: {"model","annotated","rejected","call_failures","usage"}
//
// Calls are serialised (one annotation run at a time per process): NVIDIA's free tier is
// rate-limited (~40 RPM) and best effort, so concurrent runs would only produce 429s.
func NewAnnotateHandler(client *Client) http.Handler {
	var mu sync.Mutex
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		if client == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"degraded": "no_api_key", "reason": "the reason worker has no NVIDIA_API_KEY configured"})
			return
		}
		var req AnnotateRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAnnotateBody)).Decode(&req); err != nil {
			http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		mu.Lock()
		defer mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		send := func(event string, v any) {
			b, _ := json.Marshal(v)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
			flusher.Flush()
		}
		send("started", map[string]any{"model": client.Model(), "findings": len(req.Findings)})

		var annotated, rejected, callFailures int
		var usage Usage
		err := AnnotateStream(r.Context(), client, req.Findings, func(e Event) {
			usage.PromptTokens += e.Usage.PromptTokens
			usage.CompletionTokens += e.Usage.CompletionTokens
			usage.TotalTokens += e.Usage.TotalTokens
			switch {
			case e.Annotation != nil:
				annotated++
				send("annotation", e.Annotation)
			case e.Rejection != nil:
				rejected++
				if e.Rejection.Kind == core.LLMRejectionCallFailed {
					callFailures++
				}
				send("rejected", e.Rejection)
			}
		})
		if err != nil {
			return // the caller went away; nothing more to say to it
		}
		send("done", map[string]any{"model": client.Model(), "annotated": annotated, "rejected": rejected, "call_failures": callFailures, "usage": usage})
	})
}
