// Command runnerd is P3, the experiment runner (CLAUDE.md §7): long-lived, credentialed.
// validate/ Rung 1 + Rung 3. THE ONLY PROCESS THAT EVER HOLDS CREDENTIALS.
//
// Stub: proves the process boundary. Real credential handling and the validation-ladder
// harness land in validate/ (PC-24, PC-25). Nothing here reads a credential yet — the
// point of standing up the binary now is that the boundary is structural before there is
// anything inside it worth protecting, per PC-10's Card.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"preflight/cmd/runnerd/internal/pricingfetch"
	"preflight/cmd/runnerd/internal/rung3"
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

	services := []string{"AWSELB", "AWSDataTransfer", "AmazonS3", "AmazonElastiCache", "AmazonRDS", "AmazonEC2"}
	// AWSDataTransfer carries internet egress + cross-AZ rates (PC-132, ~1.5MB/region).
	// AmazonEC2 is fetched but ONLY its NAT Gateway product family is stored (ADR-006
	// amendment, 2026-10-02): the ~441-481MB offer is streamed and everything else is dropped as
	// it is read — pricingfetch.productFamilyFilters. Every other EC2 row stays out of scope.

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

// runRung3 is PC-25: one real, ephemeral experiment. It refuses to start without an explicitly named
// sandbox account and a budget alert in it, and it always destroys what it created. Credentials are
// whatever this process inherited (AWS_* environment or profile); nothing here reads or prints them.
func runRung3(account, region, tfDir, predictions, out string, window time.Duration) {
	runID := "r3-" + time.Now().UTC().Format("20060102t150405")
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	res, err := rung3.Run(ctx, rung3.Config{
		Account: account, Region: region, TerraformDir: tfDir, PredictionsPath: predictions, RunID: runID,
		FaultWindow: window, Log: func(f string, a ...any) { log.Printf("runnerd -rung3: "+f, a...) },
	})
	if res.RunID != "" {
		b, _ := json.MarshalIndent(res, "", "  ")
		if werr := os.WriteFile(out, append(b, '\n'), 0o644); werr != nil {
			log.Printf("runnerd -rung3: could not write %s: %v", out, werr)
		} else {
			log.Printf("runnerd -rung3: result written to %s", out)
		}
		for _, c := range res.Comparison {
			log.Printf("runnerd -rung3: %s %s: %s", c.ID, c.Verdict, c.Evidence)
		}
		log.Printf("runnerd -rung3: cleanup clean=%v", res.Cleanup.Clean)
	}
	if err != nil {
		log.Fatalf("runnerd -rung3: %v", err)
	}
}

func main() {
	fetchPricing := flag.Bool("fetch-pricing", false, "fetch one real AWS pricing snapshot and store it, then exit (PC-116/ADR-006)")
	pricingDB := flag.String("pricing-db", "pricing.db", "path to the pricing SQLite store")
	pricingRegion := flag.String("pricing-region", "us-east-1", "AWS region to fetch pricing for")
	rung3Run := flag.Bool("rung3", false, "run one real ephemeral AWS experiment (apply, FIS fault, capture, destroy), then exit (PC-25)")
	rung3Account := flag.String("rung3-account", "", "REQUIRED with -rung3: the sandbox AWS account ID; the run refuses if the credentials resolve to any other")
	rung3Region := flag.String("rung3-region", "us-east-1", "AWS region for the Rung 3 experiment")
	rung3Dir := flag.String("rung3-terraform", "validate/rung3/terraform", "the experiment's Terraform directory")
	rung3Pred := flag.String("rung3-predictions", "validate/rung3/predictions.json", "the predictions committed before the run")
	rung3Out := flag.String("rung3-out", "validate/rung3/result.json", "where to write the observed result")
	rung3Window := flag.Duration("rung3-window", 150*time.Second, "how long to observe after the fault starts")
	flag.Parse()

	if *rung3Run {
		runRung3(*rung3Account, *rung3Region, *rung3Dir, *rung3Pred, *rung3Out, *rung3Window)
		return
	}

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
