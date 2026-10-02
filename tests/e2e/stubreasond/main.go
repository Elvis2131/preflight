// Command stubreasond is a STAND-IN reason worker (P2) for the browser e2e suite: it speaks the
// same SSE protocol as cmd/reasond but needs no API key and no network, and it writes canned
// narratives so the Analyze/report flows can be exercised against a real assessd. It is NOT the
// reason worker and makes no claim about model quality (that is cmd/reason-eval's job).
//
// Behaviour is switched at runtime by the contents of a control file (path in STUB_CONTROL):
// "ok" (default) streams one annotation per finding; "nokey" answers 503 degraded.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"preflight/core"
)

func main() {
	port := os.Getenv("PREFLIGHT_REASOND_PORT")
	if port == "" {
		port = "8098"
	}
	control := os.Getenv("STUB_CONTROL")
	mode := func() string {
		if control == "" {
			return "ok"
		}
		b, err := os.ReadFile(control)
		if err != nil {
			return "ok"
		}
		return strings.TrimSpace(string(b))
	}

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { fmt.Fprintln(w, "stubreasond: ok") })
	http.HandleFunc("/v1/annotate", func(w http.ResponseWriter, r *http.Request) {
		if mode() == "nokey" {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"degraded":"no_api_key"}`))
			return
		}
		var req struct{ Findings []core.Finding }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		send := func(ev string, v any) {
			b, _ := json.Marshal(v)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev, b)
			fl.Flush()
		}
		send("started", map[string]any{"model": "stub", "findings": len(req.Findings)})
		for _, f := range req.Findings {
			time.Sleep(120 * time.Millisecond) // visible streaming
			cite := f.Dimensions.AffectedComponents[0]
			send("annotation", core.LLMAnnotation{
				FindingID: f.ID, Narrative: "STUB narrative for " + f.ID, CitedEvidence: []string{cite},
				Provenance: core.NewProvenance(core.KindLLMReasoned, "stubreasond").WithReason("cites " + cite),
			})
		}
		send("done", map[string]any{})
	})
	log.Printf("stubreasond listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
