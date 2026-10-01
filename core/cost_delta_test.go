package core_test

// PC-118: ComputeCostDelta's own tests, including the ticket's own required proof —
// "verified on golden aws-broken -> aws with a hand-checked expected delta." Every
// price below is real, individually live-fetched AWS data (2026-09-25), matching
// core/cost_test.go's own realPriceTable rows plus the golden bundle's real Single-AZ
// RDS rate (aws-broken declares multi_az = false; aws declares multi_az = true —
// a real design change, verified against golden/aws-broken/rds.tf and golden/aws/rds.tf).

import (
	"path/filepath"
	"testing"

	"preflight/core"
	"preflight/ingest"
	awsprovider "preflight/providers/aws"
)

func brokenAndRealGoldenIR(t *testing.T) (broken, real *core.IR) {
	t.Helper()
	awsReg, err := awsprovider.Load()
	if err != nil {
		t.Fatalf("awsprovider.Load(): %v", err)
	}
	brokenResult, err := ingest.Ingest(filepath.Join("..", "golden", "aws-broken"), awsReg, 1)
	if err != nil {
		t.Fatalf("Ingest(aws-broken): %v", err)
	}
	if brokenResult.Insufficient != nil {
		t.Fatalf("aws-broken: got insufficient_model: %+v", brokenResult.Insufficient)
	}
	realResult, err := ingest.Ingest(filepath.Join("..", "golden", "aws"), awsReg, 2)
	if err != nil {
		t.Fatalf("Ingest(aws): %v", err)
	}
	if realResult.Insufficient != nil {
		t.Fatalf("aws: got insufficient_model: %+v", realResult.Insufficient)
	}
	return brokenResult.IR, realResult.IR
}

// realPriceTableBothDeploymentOptions carries both RDS deployment options (Single-AZ
// for aws-broken's real declared multi_az=false, Multi-AZ for aws's real declared
// multi_az=true) so both bundles resolve against the SAME snapshot — required for a
// clean, snapshot-unchanged delta.
func realPriceTableBothDeploymentOptions() core.PriceTable {
	table := realPriceTable() // core/cost_test.go's own real, live-fetched rows (Multi-AZ RDS, ElastiCache, ALB)
	table.Rows = append(table.Rows, core.PriceRow{
		Service: "AmazonRDS", Unit: "Hrs", Price: 0.45, Currency: "USD",
		SKUAttributes: map[string]string{
			"instanceType": "db.r6g.xlarge", "databaseEngine": "PostgreSQL", "deploymentOption": "Single-AZ",
			"rate_description": "$ 0.45 per RDS db.r6g.xlarge Single-AZ instance hour (or partial hour) running PostgreSQL",
		},
	})
	return table
}

func TestComputeCostDelta_GoldenBrokenToReal_HandVerified(t *testing.T) {
	brokenIR, realIR := brokenAndRealGoldenIR(t)
	prov := core.NewProvenance(core.KindDerived, "test")
	table := realPriceTableBothDeploymentOptions()

	brokenCost := core.ComputeCost(brokenIR, table, []string{"eu-west-1"}, prov)
	realCost := core.ComputeCost(realIR, table, []string{"eu-west-1"}, prov)

	entries, snapshotChanged := core.ComputeCostDelta(&brokenCost, &realCost, prov)
	if snapshotChanged {
		t.Fatal("both versions were priced against the identical PriceTable/SnapshotID — snapshotChanged must be false")
	}

	byID := map[string]core.CostDeltaEntry{}
	for _, e := range entries {
		byID[e.NodeID] = e
	}

	// RDS: aws-broken is Single-AZ ($0.45/hr), aws is Multi-AZ ($0.899/hr) — a real
	// design change (adding Multi-AZ), hand-verified: 730 * (0.899 - 0.45) = 327.77.
	rds, ok := byID["aws_db_instance.payments"]
	if !ok {
		t.Fatal("expected aws_db_instance.payments in the delta")
	}
	if rds.Kind != core.CostDeltaIncreased {
		t.Fatalf("RDS delta kind = %q, want increased: %+v", rds.Kind, rds)
	}
	wantRDSChange := (0.899 - 0.45) * core.HoursPerMonthAssumption
	if rds.ChangeAmount != wantRDSChange {
		t.Errorf("RDS ChangeAmount = %v, want %v (hand-verified: 730 * (0.899 - 0.45))", rds.ChangeAmount, wantRDSChange)
	}

	// ElastiCache: aws-broken has 1 node, aws has 3 — hand-verified:
	// 730 * 0.206 * (3-1) = 300.76.
	cache, ok := byID["aws_elasticache_replication_group.payments"]
	if !ok {
		t.Fatal("expected elasticache in the delta")
	}
	if cache.Kind != core.CostDeltaIncreased {
		t.Fatalf("ElastiCache delta kind = %q, want increased: %+v", cache.Kind, cache)
	}
	wantCacheChange := 0.206 * core.HoursPerMonthAssumption * (3 - 1)
	if cache.ChangeAmount != wantCacheChange {
		t.Errorf("ElastiCache ChangeAmount = %v, want %v (hand-verified: 730 * 0.206 * (3-1) node)", cache.ChangeAmount, wantCacheChange)
	}

	// ALB: golden/aws-broken/alb.tf and golden/aws/alb.tf are identical — unchanged.
	alb, ok := byID["aws_lb.payments"]
	if !ok {
		t.Fatal("expected alb in the delta")
	}
	if alb.Kind != core.CostDeltaUnchanged {
		t.Errorf("ALB delta kind = %q, want unchanged (identical Terraform in both bundles): %+v", alb.Kind, alb)
	}
}

func TestComputeCostDelta_DifferentSnapshots_NeverClassifiedAsDesignChange(t *testing.T) {
	prov := core.NewProvenance(core.KindDerived, "test")
	oldTable := core.PriceTable{SnapshotID: "snap-old", Rows: []core.PriceRow{
		{Service: "AWSELB", Unit: "Hrs", Price: 0.0225, SKUAttributes: map[string]string{"usagetype": "LoadBalancerUsage", "operation": "LoadBalancing:Application"}},
	}}
	newTable := core.PriceTable{SnapshotID: "snap-new", Rows: []core.PriceRow{
		// Same real SKU, but a different (hypothetical, later) list price — simulates
		// AWS's own price moving between two dated snapshots, not a design change.
		{Service: "AWSELB", Unit: "Hrs", Price: 0.03, SKUAttributes: map[string]string{"usagetype": "LoadBalancerUsage", "operation": "LoadBalancing:Application"}},
	}}

	ir := &core.IR{Nodes: []core.Node{
		{ID: "lb", Type: core.NodeTypeLoadBalancer, Resolution: core.ResolutionKnown, Provenance: prov,
			Sizing: &core.Sizing{LoadBalancerType: strPtr("application")}},
	}}

	oldCost := core.ComputeCost(ir, oldTable, []string{"eu-west-1"}, prov)
	newCost := core.ComputeCost(ir, newTable, []string{"eu-west-1"}, prov)

	entries, snapshotChanged := core.ComputeCostDelta(&oldCost, &newCost, prov)
	if !snapshotChanged {
		t.Fatal("expected snapshotChanged=true (different SnapshotIDs)")
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].Kind == core.CostDeltaIncreased || entries[0].Kind == core.CostDeltaDecreased {
		t.Fatalf("got Kind=%q — a price change between two DIFFERENT snapshots must never be classified as increased/decreased (that asserts a design change), got %+v", entries[0].Kind, entries[0])
	}
	if entries[0].Kind != core.CostDeltaPriceMovement {
		t.Errorf("Kind = %q, want CostDeltaPriceMovement", entries[0].Kind)
	}
}

func strPtr(s string) *string { return &s }
