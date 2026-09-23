// Command runnerd is P3, the experiment runner (CLAUDE.md §7): long-lived, credentialed.
// validate/ Rung 1 + Rung 3. THE ONLY PROCESS THAT EVER HOLDS CREDENTIALS.
//
// Stub: proves the process boundary. Real credential handling and the validation-ladder
// harness land in validate/ (PC-24, PC-25). Nothing here reads a credential yet — the
// point of standing up the binary now is that the boundary is structural before there is
// anything inside it worth protecting, per PC-10's Card.
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	log.Println("runnerd (P3) started — experiment runner")
	log.Println("runnerd (P3): the only process permitted to hold cloud credentials (ADR-003)")
	log.Println("runnerd (P3): stub — validate/ Rung 1/3 harness not wired up yet (see PC-24, PC-25)")

	// A real long-lived process, not select{} — select{} with no other goroutine
	// is a guaranteed Go runtime deadlock panic, not a graceful indefinite block.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("runnerd (P3): shutting down")
}
