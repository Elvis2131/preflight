// Command reasond is P2, the reason worker (CLAUDE.md §7): async, external, optional.
// Calls NVIDIA's API directly over net/http (ADR-005) to annotate an already-returned
// assessment. Can be down; the product degrades, never fails, when it is.
//
// Stub: proves the process boundary. The real HTTP client, streaming response handling,
// and I3-compliant additive-merge logic land in reason/ (PC-77's acceptance criteria).
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	log.Println("reasond (P2) started — LLM annotation worker (NVIDIA API, no SDK, ADR-005)")
	log.Println("reasond (P2): stub — no reason/ client wired up yet (see PC-77)")

	// A real long-lived process, not select{} — select{} with no other goroutine
	// is a guaranteed Go runtime deadlock panic, not a graceful indefinite block.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("reasond (P2): shutting down")
}
