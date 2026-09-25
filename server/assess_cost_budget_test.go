package server_test

// PC-118: end-to-end proof that a declared cost budget requirement (core.Workload.
// Requirements, ID "monthly_cost_budget_usd") produces a real failing compliance
// finding when hard and exceeded, and a trade-off (never a finding) when preference.

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"preflight/pricing"
	"preflight/server"
)

func writeWorkloadWithBudget(t *testing.T, priority, amount string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "workload.yaml")
	content := `
schema_version: "1.0.0"
name: payments-api
criticality: tier1
data_classification: PCI
regions:
  - eu-west-1
compliance_profiles:
  - PCI
requirements:
  - id: monthly_cost_budget_usd
    value: ` + amount + `
    priority: ` + priority + `
` + budgetRank(priority)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func budgetRank(priority string) string {
	if priority == "preference" {
		return "    rank: 1\n"
	}
	return ""
}

func expensiveActiveSnapshot() pricing.Snapshot {
	// Real RDS Multi-AZ + 3-node ElastiCache + ALB rates (core/cost_test.go's own
	// live-fetched fixture) price the golden bundle at roughly $1123/month — a low
	// budget guarantees "exceeded" without depending on exact arithmetic here.
	return pricing.Snapshot{
		ID: "expensive-snap", FetchedAt: time.Now(), Source: "test", Disclaimer: pricing.AWSDisclaimer,
		Entries: []pricing.PriceEntry{
			{Service: "AmazonRDS", Unit: "Hrs", Price: 0.899, Currency: "USD", SKUAttributes: map[string]string{"instanceType": "db.r6g.xlarge", "databaseEngine": "PostgreSQL", "deploymentOption": "Multi-AZ"}},
			// aws-broken declares multi_az = false (Single-AZ) — this row lets both
			// golden bundles resolve against the same snapshot for a clean delta.
			{Service: "AmazonRDS", Unit: "Hrs", Price: 0.45, Currency: "USD", SKUAttributes: map[string]string{"instanceType": "db.r6g.xlarge", "databaseEngine": "PostgreSQL", "deploymentOption": "Single-AZ"}},
			{Service: "AmazonElastiCache", Unit: "Hrs", Price: 0.206, Currency: "USD", SKUAttributes: map[string]string{"instanceType": "cache.r6g.large", "cacheEngine": "Redis", "usagetype": "NodeUsage:cache.r6g.large"}},
			{Service: "AWSELB", Unit: "Hrs", Price: 0.0225, Currency: "USD", SKUAttributes: map[string]string{"usagetype": "LoadBalancerUsage", "operation": "LoadBalancing:Application"}},
		},
	}
}

func TestAssess_HardBudgetExceeded_ProducesFailingFinding(t *testing.T) {
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
	if err := pricingStore.PutSnapshot(expensiveActiveSnapshot()); err != nil {
		t.Fatal(err)
	}
	if err := pricingStore.SetActive("expensive-snap"); err != nil {
		t.Fatal(err)
	}
	store.AttachPricingStore(pricingStore)

	bundleDir, err := filepath.Abs("../golden/aws")
	if err != nil {
		t.Fatal(err)
	}
	workloadPath := writeWorkloadWithBudget(t, "hard", "100")

	resp, err := server.Assess(store, server.AssessRequest{SessionID: "budget-hard", BundleDir: bundleDir, WorkloadPath: workloadPath})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if resp.Cost == nil || !resp.Cost.BudgetExceeded {
		t.Fatalf("Cost = %+v, want BudgetExceeded=true ($100 budget against a >$1000/month bundle)", resp.Cost)
	}
	if resp.Scorecard.Cost == nil {
		t.Fatal("Scorecard.Cost is nil, want the same Cost report attached as its own dimension")
	}

	found := false
	for _, f := range resp.Findings {
		if f.ID == "finding.cost.budget-compliance" {
			found = true
			status, _ := f.Outcome.Value.(string)
			if status != "unsatisfied" {
				t.Errorf("budget finding status = %q, want unsatisfied", status)
			}
		}
	}
	if !found {
		t.Fatal("expected a finding.cost.budget-compliance finding when a hard budget is exceeded")
	}
}

func TestAssess_PreferenceBudgetExceeded_NoFindingButTradeoffRecorded(t *testing.T) {
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
	if err := pricingStore.PutSnapshot(expensiveActiveSnapshot()); err != nil {
		t.Fatal(err)
	}
	if err := pricingStore.SetActive("expensive-snap"); err != nil {
		t.Fatal(err)
	}
	store.AttachPricingStore(pricingStore)

	bundleDir, err := filepath.Abs("../golden/aws")
	if err != nil {
		t.Fatal(err)
	}
	workloadPath := writeWorkloadWithBudget(t, "preference", "100")

	resp, err := server.Assess(store, server.AssessRequest{SessionID: "budget-pref", BundleDir: bundleDir, WorkloadPath: workloadPath})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if resp.Cost == nil || !resp.Cost.BudgetExceeded {
		t.Fatalf("Cost = %+v, want BudgetExceeded=true (the trade-off fact must still be recorded)", resp.Cost)
	}
	for _, f := range resp.Findings {
		if f.ID == "finding.cost.budget-compliance" {
			t.Fatalf("got a finding.cost.budget-compliance finding for a PREFERENCE budget — must never happen, got %+v", f)
		}
	}
}

func TestAssess_CostDeltaAcrossVersions(t *testing.T) {
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
	if err := pricingStore.PutSnapshot(expensiveActiveSnapshot()); err != nil {
		t.Fatal(err)
	}
	if err := pricingStore.SetActive("expensive-snap"); err != nil {
		t.Fatal(err)
	}
	store.AttachPricingStore(pricingStore)

	brokenDir, err := filepath.Abs("../golden/aws-broken")
	if err != nil {
		t.Fatal(err)
	}
	realDir, err := filepath.Abs("../golden/aws")
	if err != nil {
		t.Fatal(err)
	}
	workloadPath, err := filepath.Abs("../golden/workload.yaml")
	if err != nil {
		t.Fatal(err)
	}

	v1, err := server.Assess(store, server.AssessRequest{SessionID: "cost-delta-e2e", BundleDir: brokenDir, WorkloadPath: workloadPath})
	if err != nil {
		t.Fatalf("Assess v1: %v", err)
	}
	if v1.CostDelta != nil {
		t.Errorf("v1.CostDelta = %+v, want nil (first version, nothing to diff)", v1.CostDelta)
	}

	v2, err := server.Assess(store, server.AssessRequest{SessionID: "cost-delta-e2e", BundleDir: realDir, WorkloadPath: workloadPath})
	if err != nil {
		t.Fatalf("Assess v2: %v", err)
	}
	if v2.CostSnapshotChanged {
		t.Error("CostSnapshotChanged = true, want false (same active snapshot both times)")
	}
	if len(v2.CostDelta) == 0 {
		t.Fatal("expected real cost delta entries between aws-broken and aws")
	}
	foundIncrease := false
	for _, e := range v2.CostDelta {
		if e.NodeID == "aws_db_instance.payments" && e.ChangeAmount > 0 {
			foundIncrease = true
		}
	}
	if !foundIncrease {
		t.Error("expected the RDS component to show a cost increase (aws-broken is Single-AZ, aws is Multi-AZ)")
	}
}
