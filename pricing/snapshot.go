// Package pricing holds PC-116's pricing snapshot data model and SQLite-backed
// storage — deliberately zero network code (verified by boundary_test.go): the real
// AWS Bulk Price List API fetcher lives in cmd/runnerd/internal/pricingfetch,
// compiler-private to cmd/runnerd (ADR-006 §2), so this package is safe for the
// assessment engine (server, P1) to import for the read-only /pricing/snapshots
// endpoints without pulling any network capability into P1 at all.
package pricing

import "time"

// PriceEntry is one normalized, looked-up-able price fact — ADR-006 §4's own shape.
type PriceEntry struct {
	Service       string            `json:"service"`
	Region        string            `json:"region"`
	SKUAttributes map[string]string `json:"sku_attributes"`
	Unit          string            `json:"unit"`
	Price         float64           `json:"price"`
	Currency      string            `json:"currency"`
}

// Snapshot is one fetch cycle's full, dated result — ADR-006 §4.
type Snapshot struct {
	ID         string       `json:"id"`
	FetchedAt  time.Time    `json:"fetched_at"`
	Source     string       `json:"source"` // e.g. "AWS Bulk Price List API, AmazonRDS 20260924211011"
	Disclaimer string       `json:"disclaimer"`
	Entries    []PriceEntry `json:"entries"`
	Active     bool         `json:"active"`
}

// AWSDisclaimer is AWS's own real, verbatim disclaimer — read directly from the
// Bulk API's own "disclaimer" field (e.g. AmazonElastiCache's region_index.json,
// fetched and inspected 2026-09-25), not paraphrased. Never omitted from a report
// that carries a cost derived from a snapshot.
const AWSDisclaimer = "This pricing list is for informational purposes only. All prices are subject to the additional terms included in the pricing pages on http://aws.amazon.com. All Free Tier prices are also subject to the terms included at https://aws.amazon.com/free/"
