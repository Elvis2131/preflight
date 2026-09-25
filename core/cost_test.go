package core_test

// PC-117: ComputeCost's own tests. The PriceTable rows below are REAL data —
// individually fetched live from AWS's own Bulk Price List API and hand-verified
// (2026-09-25), not invented: RDS db.r6g.xlarge/PostgreSQL/Multi-AZ ($0.899/hr),
// ElastiCache cache.r6g.large/Redis ($0.206/hr), and the base ALB hourly rate
// ($0.0225/hr) — the exact SKUs/rateCodes the golden AWS bundle's own real sizing
// (PC-115) should match against.

import (
	"path/filepath"
	"testing"

	"preflight/core"
	"preflight/ingest"
	awsprovider "preflight/providers/aws"
)

func realPriceTable() core.PriceTable {
	return core.PriceTable{
		SnapshotID: "test-snapshot-2026-09-25",
		Rows: []core.PriceRow{
			{
				Service: "AmazonRDS", Unit: "Hrs", Price: 0.899, Currency: "USD",
				SKUAttributes: map[string]string{
					"instanceType": "db.r6g.xlarge", "databaseEngine": "PostgreSQL", "deploymentOption": "Multi-AZ",
					"rate_description": "$ 0.899 per RDS db.r6g.xlarge Multi-AZ instance hour (or partial hour) running PostgreSQL",
				},
			},
			{
				Service: "AmazonElastiCache", Unit: "Hrs", Price: 0.206, Currency: "USD",
				SKUAttributes: map[string]string{
					"instanceType": "cache.r6g.large", "cacheEngine": "Redis", "usagetype": "NodeUsage:cache.r6g.large",
					"rate_description": "$0.206 per Memory optimized r6g.large node hour running Redis",
				},
			},
			{
				Service: "AWSELB", Unit: "Hrs", Price: 0.0225, Currency: "USD",
				SKUAttributes: map[string]string{
					"usagetype": "LoadBalancerUsage", "operation": "LoadBalancing:Application",
					"rate_description": "$0.0225 per Application LoadBalancer-hour (or partial hour)",
				},
			},
		},
	}
}

func realGoldenIR(t *testing.T) *core.IR {
	t.Helper()
	awsReg, err := awsprovider.Load()
	if err != nil {
		t.Fatalf("awsprovider.Load(): %v", err)
	}
	result, err := ingest.Ingest(filepath.Join("..", "golden", "aws"), awsReg, 1)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if result.Insufficient != nil {
		t.Fatalf("got insufficient_model: %+v", result.Insufficient)
	}
	return result.IR
}

func TestComputeCost_GoldenBundle_HandVerifiedAgainstRealPriceRows(t *testing.T) {
	ir := realGoldenIR(t)
	prov := core.NewProvenance(core.KindDerived, "test")
	report := core.ComputeCost(ir, realPriceTable(), prov)

	if report.SnapshotID != "test-snapshot-2026-09-25" {
		t.Errorf("SnapshotID = %q", report.SnapshotID)
	}
	if report.HoursPerMonthAssumed != core.HoursPerMonthAssumption {
		t.Errorf("HoursPerMonthAssumed = %v, want %v", report.HoursPerMonthAssumed, core.HoursPerMonthAssumption)
	}

	byID := map[string]core.ComponentCost{}
	for _, c := range report.Components {
		byID[c.NodeID] = c
	}

	rds, ok := byID["aws_db_instance.payments"]
	if !ok || rds.Decision != core.CostPriced {
		t.Fatalf("aws_db_instance.payments: got %+v, want priced", rds)
	}
	wantRDS := 0.899 * core.HoursPerMonthAssumption
	if rds.MonthlyAmount != wantRDS {
		t.Errorf("RDS MonthlyAmount = %v, want %v (hand-verified: $0.899/hr x 730)", rds.MonthlyAmount, wantRDS)
	}

	cache, ok := byID["aws_elasticache_replication_group.payments"]
	if !ok || cache.Decision != core.CostPriced {
		t.Fatalf("elasticache: got %+v, want priced", cache)
	}
	wantCache := 0.206 * core.HoursPerMonthAssumption
	if cache.MonthlyAmount != wantCache {
		t.Errorf("ElastiCache MonthlyAmount = %v, want %v (hand-verified: $0.206/hr x 730)", cache.MonthlyAmount, wantCache)
	}

	alb, ok := byID["aws_lb.payments"]
	if !ok || alb.Decision != core.CostPriced {
		t.Fatalf("alb: got %+v, want priced", alb)
	}
	wantALB := 0.0225 * core.HoursPerMonthAssumption
	if alb.MonthlyAmount != wantALB {
		t.Errorf("ALB MonthlyAmount = %v, want %v (hand-verified: $0.0225/hr x 730)", alb.MonthlyAmount, wantALB)
	}

	wantTotal := wantRDS + wantCache + wantALB
	if report.PricedTotal != wantTotal {
		t.Errorf("PricedTotal = %v, want %v (sum of the three hand-verified priced components)", report.PricedTotal, wantTotal)
	}
}

// TestComputeCost_UnpricedComponentNeverCountsAsZero is PC-117's own acceptance
// criterion, directly: an EKS node has no RDS/ElastiCache/ALB-shaped price row at all
// in this scope (EC2/EKS pricing is deferred past v1, ADR-006 §3) — it must show up
// as cost_unknown with a real reason, and PricedTotal must not silently include it as
// 0 (which would be indistinguishable from "verified free").
func TestComputeCost_UnpricedComponentNeverCountsAsZero(t *testing.T) {
	ir := realGoldenIR(t)
	prov := core.NewProvenance(core.KindDerived, "test")
	report := core.ComputeCost(ir, realPriceTable(), prov)

	var eks *core.ComponentCost
	for i := range report.Components {
		if report.Components[i].NodeID == "aws_eks_cluster.payments" {
			eks = &report.Components[i]
		}
	}
	if eks == nil {
		t.Fatal("expected aws_eks_cluster.payments in the component list")
	}
	if eks.Decision != core.CostUnknown {
		t.Errorf("eks cluster: got %+v, want cost_unknown (EC2/EKS pricing is deferred, ADR-006 §3)", eks)
	}
	if eks.Reason == "" {
		t.Error("eks cluster: cost_unknown with no reason — must always state why")
	}
	if eks.MonthlyAmount != 0 {
		t.Errorf("eks cluster: MonthlyAmount = %v, want 0 (unpriced, not a computed zero)", eks.MonthlyAmount)
	}
	if report.UnpricedCount == 0 {
		t.Error("UnpricedCount = 0, want at least 1 (the EKS cluster, plus any other unpriced components)")
	}

	// The core assertion: PricedTotal excludes the unpriced component entirely — it
	// is not silently folded in as a $0 contribution.
	var sumOfPricedOnly float64
	for _, c := range report.Components {
		if c.Decision == core.CostPriced {
			sumOfPricedOnly += c.MonthlyAmount
		}
	}
	if report.PricedTotal != sumOfPricedOnly {
		t.Errorf("PricedTotal = %v, want exactly the sum of CostPriced components (%v) — never including unpriced ones", report.PricedTotal, sumOfPricedOnly)
	}
}

func TestComputeCost_Deterministic(t *testing.T) {
	ir := realGoldenIR(t)
	prov := core.NewProvenance(core.KindDerived, "test")
	table := realPriceTable()

	r1 := core.ComputeCost(ir, table, prov)
	r2 := core.ComputeCost(ir, table, prov)

	if len(r1.Components) != len(r2.Components) {
		t.Fatalf("component count differs: %d vs %d", len(r1.Components), len(r2.Components))
	}
	for i := range r1.Components {
		if r1.Components[i] != r2.Components[i] {
			t.Fatalf("component %d differs between identical runs: %+v vs %+v", i, r1.Components[i], r2.Components[i])
		}
	}
	if r1.PricedTotal != r2.PricedTotal {
		t.Fatalf("PricedTotal differs between identical runs: %v vs %v", r1.PricedTotal, r2.PricedTotal)
	}
}

func TestComputeCost_MissingSizing_IsCostUnknown(t *testing.T) {
	prov := core.NewProvenance(core.KindDerived, "test")
	ir := &core.IR{Nodes: []core.Node{
		{ID: "n1", Type: core.NodeTypeManagedDatabase, Resolution: core.ResolutionKnown, Provenance: prov},
	}}
	report := core.ComputeCost(ir, realPriceTable(), prov)
	if len(report.Components) != 1 || report.Components[0].Decision != core.CostUnknown {
		t.Fatalf("got %+v, want a single cost_unknown component (no Sizing at all)", report.Components)
	}
	if report.Components[0].Reason == "" {
		t.Error("expected a real reason naming the missing sizing, got empty string")
	}
}
