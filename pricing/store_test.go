package pricing_test

// PC-116's own acceptance criterion, directly: "Two snapshots with different
// fetched-at times coexist; an assessment pinned to one is unaffected by the other."

import (
	"testing"
	"time"

	"preflight/pricing"
)

func mustOpen(t *testing.T) *pricing.Store {
	t.Helper()
	s, err := pricing.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStore_TwoSnapshotsCoexistIndependently(t *testing.T) {
	s := mustOpen(t)

	old := pricing.Snapshot{
		ID:         "snap-old",
		FetchedAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Source:     "AWS Bulk Price List API, AmazonRDS 20260101000000",
		Disclaimer: pricing.AWSDisclaimer,
		Entries: []pricing.PriceEntry{
			{Service: "AmazonRDS", Region: "us-east-1", SKUAttributes: map[string]string{"instanceType": "db.r6g.xlarge"}, Unit: "Hrs", Price: 1.0, Currency: "USD"},
		},
	}
	newer := pricing.Snapshot{
		ID:         "snap-new",
		FetchedAt:  time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		Source:     "AWS Bulk Price List API, AmazonRDS 20260901000000",
		Disclaimer: pricing.AWSDisclaimer,
		Entries: []pricing.PriceEntry{
			{Service: "AmazonRDS", Region: "us-east-1", SKUAttributes: map[string]string{"instanceType": "db.r6g.xlarge"}, Unit: "Hrs", Price: 1.5, Currency: "USD"},
		},
	}

	if err := s.PutSnapshot(old); err != nil {
		t.Fatalf("PutSnapshot(old): %v", err)
	}
	if err := s.PutSnapshot(newer); err != nil {
		t.Fatalf("PutSnapshot(newer): %v", err)
	}

	list, err := s.ListSnapshots()
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d snapshots, want 2", len(list))
	}

	gotOld, ok, err := s.GetSnapshot("snap-old")
	if err != nil || !ok {
		t.Fatalf("GetSnapshot(snap-old): ok=%v err=%v", ok, err)
	}
	if gotOld.Entries[0].Price != 1.0 {
		t.Errorf("snap-old price = %v, want 1.0 (unaffected by snap-new)", gotOld.Entries[0].Price)
	}

	gotNew, ok, err := s.GetSnapshot("snap-new")
	if err != nil || !ok {
		t.Fatalf("GetSnapshot(snap-new): ok=%v err=%v", ok, err)
	}
	if gotNew.Entries[0].Price != 1.5 {
		t.Errorf("snap-new price = %v, want 1.5 (unaffected by snap-old)", gotNew.Entries[0].Price)
	}
}

func TestStore_SetActive_ExactlyOneActiveAtATime(t *testing.T) {
	s := mustOpen(t)
	a := pricing.Snapshot{ID: "a", FetchedAt: time.Now(), Source: "x", Disclaimer: pricing.AWSDisclaimer}
	b := pricing.Snapshot{ID: "b", FetchedAt: time.Now(), Source: "x", Disclaimer: pricing.AWSDisclaimer}
	if err := s.PutSnapshot(a); err != nil {
		t.Fatal(err)
	}
	if err := s.PutSnapshot(b); err != nil {
		t.Fatal(err)
	}

	if err := s.SetActive("a"); err != nil {
		t.Fatalf("SetActive(a): %v", err)
	}
	active, ok, err := s.ActiveSnapshot()
	if err != nil || !ok || active.ID != "a" {
		t.Fatalf("ActiveSnapshot: got %+v ok=%v err=%v, want a", active, ok, err)
	}

	if err := s.SetActive("b"); err != nil {
		t.Fatalf("SetActive(b): %v", err)
	}
	active, ok, err = s.ActiveSnapshot()
	if err != nil || !ok || active.ID != "b" {
		t.Fatalf("ActiveSnapshot: got %+v ok=%v err=%v, want b (a's activation must be cleared)", active, ok, err)
	}
}

func TestStore_SetActive_UnknownSnapshotRejected(t *testing.T) {
	s := mustOpen(t)
	if err := s.SetActive("does-not-exist"); err == nil {
		t.Fatal("expected an error activating a snapshot that was never stored")
	}
}

func TestStore_GetSnapshot_NotFound(t *testing.T) {
	s := mustOpen(t)
	_, ok, err := s.GetSnapshot("nope")
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for a snapshot that was never stored")
	}
}
