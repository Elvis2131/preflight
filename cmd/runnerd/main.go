// Command runnerd is P3, the experiment runner (CLAUDE.md §7): long-lived, credentialed.
// validate/ Rung 1 + Rung 3. THE ONLY PROCESS THAT EVER HOLDS CREDENTIALS.
//
// Stub: proves the process boundary. Real credential handling and the validation-ladder
// harness land in validate/ (PC-24, PC-25). Nothing here reads a credential yet — the
// point of standing up the binary now is that the boundary is structural before there is
// anything inside it worth protecting, per PC-10's Card.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"preflight/cmd/runnerd/internal/pricingfetch"
	"preflight/pricing"
)

// runFetchPricing is PC-116/ADR-006's one-shot pricing-snapshot fetch subcommand: a
// cron-invoked, exit-when-done job (`runnerd -fetch-pricing -pricing-db path.db`),
// not part of the long-running experiment-runner main loop below — the same "outside
// the assessment path, own cadence" property a fourth process would have, without
// adding one (ADR-006 §2's own rejected-alternative reasoning).
func runFetchPricing(dbPath, region string) {
	store, err := pricing.OpenStore(dbPath)
	if err != nil {
		log.Fatalf("runnerd -fetch-pricing: open pricing store: %v", err)
	}
	defer store.Close()

	services := []string{"AWSELB", "AmazonS3", "AmazonElastiCache", "AmazonRDS"}
	// EC2 and NAT Gateway deliberately excluded — ADR-006 §3: the real, measured
	// 481MB/region EC2 offer file is deferred past v1, not silently dropped.

	client := &http.Client{Timeout: 5 * time.Minute}
	snapshotID := time.Now().UTC().Format("20060102T150405Z")
	snap, err := pricingfetch.FetchSnapshot(client, snapshotID, services, region)
	if err != nil {
		log.Fatalf("runnerd -fetch-pricing: %v", err)
	}
	if err := store.PutSnapshot(snap); err != nil {
		log.Fatalf("runnerd -fetch-pricing: store snapshot: %v", err)
	}
	log.Printf("runnerd -fetch-pricing: stored snapshot %s (%d entries) into %s", snap.ID, len(snap.Entries), dbPath)
}

func main() {
	fetchPricing := flag.Bool("fetch-pricing", false, "fetch one real AWS pricing snapshot and store it, then exit (PC-116/ADR-006)")
	pricingDB := flag.String("pricing-db", "pricing.db", "path to the pricing SQLite store")
	pricingRegion := flag.String("pricing-region", "us-east-1", "AWS region to fetch pricing for")
	flag.Parse()

	if *fetchPricing {
		runFetchPricing(*pricingDB, *pricingRegion)
		return
	}

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
