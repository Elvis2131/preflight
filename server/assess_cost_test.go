package server_test

// PC-117's own server-wiring half: proves Assess actually computes and persists Cost
// when a pricing.Store is attached, that a pinned snapshot ID is honored, that the
// active snapshot is the default, and that the GET read-back exposes the same Cost —
// this ticket's own stated acceptance criterion, made real.

import (
	"path/filepath"
	"testing"
	"time"

	"preflight/pricing"
	"preflight/server"
)

func realPricingSnapshot(id string) pricing.Snapshot {
	return pricing.Snapshot{
		ID: id, FetchedAt: time.Now(), Source: "test", Disclaimer: pricing.AWSDisclaimer,
		Entries: []pricing.PriceEntry{
			{
				Service: "AmazonRDS", Unit: "Hrs", Price: 0.899, Currency: "USD",
				SKUAttributes: map[string]string{"instanceType": "db.r6g.xlarge", "databaseEngine": "PostgreSQL", "deploymentOption": "Multi-AZ"},
			},
			{
				Service: "AmazonElastiCache", Unit: "Hrs", Price: 0.206, Currency: "USD",
				SKUAttributes: map[string]string{"instanceType": "cache.r6g.large", "cacheEngine": "Redis", "usagetype": "NodeUsage:cache.r6g.large"},
			},
			{
				Service: "AWSELB", Unit: "Hrs", Price: 0.0225, Currency: "USD",
				SKUAttributes: map[string]string{"usagetype": "LoadBalancerUsage", "operation": "LoadBalancing:Application"},
			},
		},
	}
}

func TestAssess_ComputesCostAgainstActiveSnapshot(t *testing.T) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	pricingStore, err := pricing.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("pricing.OpenStore: %v", err)
	}
	defer pricingStore.Close()
	if err := pricingStore.PutSnapshot(realPricingSnapshot("active-snap")); err != nil {
		t.Fatalf("PutSnapshot: %v", err)
	}
	if err := pricingStore.SetActive("active-snap"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	store.AttachPricingStore(pricingStore)

	bundleDir, err := filepath.Abs("../golden/aws")
	if err != nil {
		t.Fatal(err)
	}
	workloadPath, err := filepath.Abs("../golden/workload.yaml")
	if err != nil {
		t.Fatal(err)
	}

	resp, err := server.Assess(store, server.AssessRequest{
		SessionID: "cost-test-active", BundleDir: bundleDir, WorkloadPath: workloadPath,
	})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if resp.Cost == nil {
		t.Fatal("expected a real Cost report using the active snapshot, got nil")
	}
	if resp.Cost.SnapshotID != "active-snap" {
		t.Errorf("Cost.SnapshotID = %q, want active-snap (the default/active snapshot)", resp.Cost.SnapshotID)
	}
	if resp.Cost.PricedTotal <= 0 {
		t.Errorf("Cost.PricedTotal = %v, want > 0 (RDS/ElastiCache/ALB all price against this fixture)", resp.Cost.PricedTotal)
	}

	// Read-back must expose the identical Cost.
	readBack, err := server.GetStoredVersion(store, "cost-test-active", resp.VersionNumber)
	if err != nil {
		t.Fatalf("GetStoredVersion: %v", err)
	}
	if readBack.Cost == nil || readBack.Cost.SnapshotID != "active-snap" {
		t.Fatalf("read-back Cost = %+v, want the same active-snap report", readBack.Cost)
	}
	if readBack.Cost.PricedTotal != resp.Cost.PricedTotal {
		t.Errorf("read-back PricedTotal = %v, want %v (byte-identical to the original assess)", readBack.Cost.PricedTotal, resp.Cost.PricedTotal)
	}
}

func TestAssess_PinnedSnapshotOverridesActive(t *testing.T) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	pricingStore, err := pricing.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("pricing.OpenStore: %v", err)
	}
	defer pricingStore.Close()
	if err := pricingStore.PutSnapshot(realPricingSnapshot("old-snap")); err != nil {
		t.Fatal(err)
	}
	if err := pricingStore.SetActive("old-snap"); err != nil {
		t.Fatal(err)
	}
	pinned := realPricingSnapshot("pinned-snap")
	// Change one price so the two snapshots are distinguishable in the result.
	pinned.Entries[0].Price = 1.5
	if err := pricingStore.PutSnapshot(pinned); err != nil {
		t.Fatal(err)
	}
	store.AttachPricingStore(pricingStore)

	bundleDir, err := filepath.Abs("../golden/aws")
	if err != nil {
		t.Fatal(err)
	}
	workloadPath, err := filepath.Abs("../golden/workload.yaml")
	if err != nil {
		t.Fatal(err)
	}

	resp, err := server.Assess(store, server.AssessRequest{
		SessionID: "cost-test-pinned", BundleDir: bundleDir, WorkloadPath: workloadPath,
		PriceSnapshotID: "pinned-snap",
	})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if resp.Cost == nil || resp.Cost.SnapshotID != "pinned-snap" {
		t.Fatalf("Cost = %+v, want the explicitly pinned snapshot, not the active one", resp.Cost)
	}
}

func TestAssess_UnknownPinnedSnapshot_Returns404(t *testing.T) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()
	pricingStore, err := pricing.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("pricing.OpenStore: %v", err)
	}
	defer pricingStore.Close()
	store.AttachPricingStore(pricingStore)

	bundleDir, err := filepath.Abs("../golden/aws")
	if err != nil {
		t.Fatal(err)
	}
	workloadPath, err := filepath.Abs("../golden/workload.yaml")
	if err != nil {
		t.Fatal(err)
	}

	_, err = server.Assess(store, server.AssessRequest{
		SessionID: "cost-test-missing", BundleDir: bundleDir, WorkloadPath: workloadPath,
		PriceSnapshotID: "does-not-exist",
	})
	var apiErr *server.APIError
	if err == nil {
		t.Fatal("expected an error pinning a snapshot that was never stored")
	}
	if !isAPIError(err, &apiErr) || apiErr.Code != "snapshot_not_found" {
		t.Fatalf("got %v, want a snapshot_not_found APIError", err)
	}
}

func TestAssess_NoPricingStoreAttached_CostIsNil(t *testing.T) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()
	// Deliberately no AttachPricingStore call — every pre-PC-117 caller's behavior.

	bundleDir, err := filepath.Abs("../golden/aws")
	if err != nil {
		t.Fatal(err)
	}
	workloadPath, err := filepath.Abs("../golden/workload.yaml")
	if err != nil {
		t.Fatal(err)
	}

	resp, err := server.Assess(store, server.AssessRequest{
		SessionID: "cost-test-none", BundleDir: bundleDir, WorkloadPath: workloadPath,
	})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if resp.Cost != nil {
		t.Fatalf("Cost = %+v, want nil when no pricing store is attached", resp.Cost)
	}
}

func isAPIError(err error, target **server.APIError) bool {
	ae, ok := err.(*server.APIError)
	if !ok {
		return false
	}
	*target = ae
	return true
}
