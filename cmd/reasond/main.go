// Command reasond is P2, the reason worker (CLAUDE.md §7): async, external, optional.
// Calls NVIDIA's API directly over net/http (ADR-005) to annotate an already-returned
// assessment. Can be down; the product degrades, never fails, when it is.
//
// The SDK-free client, the stream parser and the I3-safe annotator live in reason/
// (PC-77). What this process still lacks is a job source — something that hands it a
// frozen assessment to annotate — which belongs to the assess/MCP wiring (PC-21), so it
// stays a process-boundary stub until then.
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	log.Println("reasond (P2) started — LLM annotation worker (NVIDIA API, no SDK, ADR-005)")
	log.Println("reasond (P2): reason/ client exists (PC-77); no job source is wired up yet (see PC-21)")

	// A real long-lived process, not select{} — select{} with no other goroutine
	// is a guaranteed Go runtime deadlock panic, not a graceful indefinite block.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("reasond (P2): shutting down")
}
