// Command seedpricing writes one active pricing snapshot into a pricing store, for the
// browser end-to-end suite (canvas/e2e). Its entries are the same real-shaped rows the
// server's own cost tests use (RDS db.r6g.xlarge, ElastiCache cache.r6g.large, the base ALB
// hourly rate) — a fixture snapshot, so verifying report/re-price needs NO live AWS pricing
// and no credentials (PC-122's decision comment).
//
// Run with: go run ./tests/e2e/seedpricing <pricing-db-path>
package main

import (
	"fmt"
	"os"
	"time"

	"preflight/pricing"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: seedpricing <pricing-db-path>")
		os.Exit(2)
	}
	st, err := pricing.OpenStore(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "seedpricing:", err)
		os.Exit(1)
	}
	defer st.Close()

	snap := pricing.Snapshot{
		ID: "e2e-fixture-snapshot", FetchedAt: time.Now(), Source: "fixture snapshot for the browser e2e suite (not a live AWS pull)",
		Disclaimer: pricing.AWSDisclaimer,
		Entries: []pricing.PriceEntry{
			{Service: "AmazonRDS", Unit: "Hrs", Price: 0.899, Currency: "USD", SKUAttributes: map[string]string{"instanceType": "db.r6g.xlarge", "databaseEngine": "PostgreSQL", "deploymentOption": "Multi-AZ"}},
			{Service: "AmazonElastiCache", Unit: "Hrs", Price: 0.206, Currency: "USD", SKUAttributes: map[string]string{"instanceType": "cache.r6g.large", "cacheEngine": "Redis", "usagetype": "NodeUsage:cache.r6g.large"}},
			{Service: "AWSELB", Unit: "Hrs", Price: 0.0225, Currency: "USD", SKUAttributes: map[string]string{"usagetype": "LoadBalancerUsage", "operation": "LoadBalancing:Application"}},
		},
	}
	if err := st.PutSnapshot(snap); err != nil {
		fmt.Fprintln(os.Stderr, "seedpricing:", err)
		os.Exit(1)
	}
	if err := st.SetActive(snap.ID); err != nil {
		fmt.Fprintln(os.Stderr, "seedpricing:", err)
		os.Exit(1)
	}
}
