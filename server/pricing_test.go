package server_test

// PC-116: proves the read-only pricing endpoints against real pricing.Store data —
// no network call involved (pricing.Store itself has zero network capability,
// pricing/boundary_test.go).

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"preflight/pricing"
	"preflight/server"
)

func TestPricingHandlers_ListGetActivate(t *testing.T) {
	store, err := pricing.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	snap := pricing.Snapshot{
		ID:         "snap-1",
		FetchedAt:  time.Now(),
		Source:     "AWS Bulk Price List API, AWSELB 20260911124544",
		Disclaimer: pricing.AWSDisclaimer,
		Entries: []pricing.PriceEntry{
			{Service: "AWSELB", Region: "us-east-1", SKUAttributes: map[string]string{"usagetype": "DataProcessing-Bytes"}, Unit: "GB", Price: 0.008, Currency: "USD"},
		},
	}
	if err := store.PutSnapshot(snap); err != nil {
		t.Fatalf("PutSnapshot: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /pricing/snapshots", server.ListPricingSnapshotsHandler(store))
	mux.HandleFunc("GET /pricing/snapshots/{id}", server.GetPricingSnapshotHandler(store))
	mux.HandleFunc("POST /pricing/snapshots/{id}/activate", server.ActivatePricingSnapshotHandler(store))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/pricing/snapshots")
	if err != nil {
		t.Fatalf("GET /pricing/snapshots: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /pricing/snapshots: status %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp, err = http.Get(ts.URL + "/pricing/snapshots/snap-1")
	if err != nil {
		t.Fatalf("GET /pricing/snapshots/snap-1: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /pricing/snapshots/snap-1: status %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp, err = http.Get(ts.URL + "/pricing/snapshots/does-not-exist")
	if err != nil {
		t.Fatalf("GET /pricing/snapshots/does-not-exist: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET /pricing/snapshots/does-not-exist: status %d, want 404", resp.StatusCode)
	}
	resp.Body.Close()

	resp, err = http.Post(ts.URL+"/pricing/snapshots/snap-1/activate", "application/json", nil)
	if err != nil {
		t.Fatalf("POST activate: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("POST activate: status %d, want 204", resp.StatusCode)
	}
	resp.Body.Close()

	active, ok, err := store.ActiveSnapshot()
	if err != nil || !ok || active.ID != "snap-1" {
		t.Fatalf("ActiveSnapshot after activation: got %+v ok=%v err=%v", active, ok, err)
	}
}
