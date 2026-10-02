// Command reasond is P2, the reason worker (CLAUDE.md §7, ADR-003): async, external, optional.
// It is the ONLY process that reads NVIDIA_API_KEY and the only one that calls NVIDIA's API
// (ADR-005, direct net/http, no SDK). It receives an already-returned set of findings from P1
// and streams back llm_reasoned annotations over SSE; it can be down, rate-limited or keyless
// and the product degrades — the deterministic assessment is never blocked or changed by it.
//
//	NVIDIA_API_KEY=...  PREFLIGHT_REASOND_PORT=8098  reasond
//
// Without a key it still starts and answers POST /v1/annotate with 503 {"degraded":"no_api_key"}.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"preflight/reason"
)

// defaultModel is the model ADR-005 / PC-77 evaluated.
const defaultModel = "nvidia/nemotron-3-super-120b-a12b"

func main() {
	port := os.Getenv("PREFLIGHT_REASOND_PORT")
	if port == "" {
		port = "8098"
	}
	model := os.Getenv("PREFLIGHT_REASON_MODEL")
	if model == "" {
		model = defaultModel
	}

	var client *reason.Client
	if key := os.Getenv("NVIDIA_API_KEY"); key != "" {
		c, err := reason.NewClient(reason.Config{APIKey: key, Model: model, Reasoning: os.Getenv("PREFLIGHT_REASON_REASONING")})
		if err != nil {
			log.Fatalf("reasond: %v", err)
		}
		client = c
		log.Printf("reasond (P2): annotating with %s", model)
	} else {
		log.Println("reasond (P2): NVIDIA_API_KEY is not set — /v1/annotate will answer 503 degraded (no_api_key); the product keeps working without narratives")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "reasond (P2): ok — key configured: %v\n", client != nil)
	})
	mux.Handle("/v1/annotate", reason.NewAnnotateHandler(client))

	srv := &http.Server{Addr: ":" + port, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		log.Printf("reasond (P2) listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("reasond: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("reasond (P2): shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
