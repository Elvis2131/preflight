//go:build live_pricing

package pricingfetch

// Deliberately gated behind a build tag, not part of the default `go test ./...` run
// — the same "external dependency verified deliberately, never on every commit"
// discipline this codebase already established for cmd/check-conformance-links (a
// weekly job, not a per-commit one): a real network call to AWS's own live endpoint
// has no place failing an unrelated commit if AWS ever reshapes a URL. Run manually
// with `go test -tags live_pricing ./cmd/runnerd/internal/pricingfetch/...` to prove
// genuine, current connectivity against the real Bulk API — this is what actually
// verified ADR-006's own live findings (file sizes, real JSON shapes, disclaimer text)
// before any fixture was captured.

import (
	"net/http"
	"testing"
	"time"
)

func TestFetch_LiveAWSELB_RealNetworkCall(t *testing.T) {
	client := &http.Client{Timeout: 30 * time.Second}
	entries, source, err := Fetch(client, "AWSELB", "us-east-1")
	if err != nil {
		t.Fatalf("live Fetch against AWS's real endpoint failed: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("live fetch returned zero entries")
	}
	t.Logf("live fetch: %d entries, source=%s", len(entries), source)
}

// ADR-006 amendment: the real ~441-481 MB AmazonEC2 offer is streamed and only the NAT Gateway
// family is kept. Needs ~450 MB of download; run deliberately:
//
//	go test -tags live_pricing ./cmd/runnerd/internal/pricingfetch/ -run EC2 -v
func TestFetch_LiveAmazonEC2_NATOnly_RealNetworkCall(t *testing.T) {
	client := &http.Client{Timeout: 10 * time.Minute}
	start := time.Now()
	entries, source, err := Fetch(client, "AmazonEC2", "eu-west-1")
	if err != nil {
		t.Fatalf("live EC2 fetch failed: %v", err)
	}
	if len(entries) < 4 {
		t.Fatalf("got %d NAT rows, want at least the zonal hours/bytes pair", len(entries))
	}
	for _, e := range entries {
		if e.SKUAttributes["product_family"] != "NAT Gateway" {
			t.Fatalf("non-NAT row stored: %+v", e)
		}
	}
	t.Logf("live EC2 fetch: %d NAT rows in %s; source=%s", len(entries), time.Since(start).Round(time.Second), source)
}
