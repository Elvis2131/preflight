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
