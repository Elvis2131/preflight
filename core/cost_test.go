package core_test

// PC-117: ComputeCost's own tests. The PriceTable rows below are REAL data —
// individually fetched live from AWS's own Bulk Price List API and hand-verified
// (2026-09-25), not invented: RDS db.r6g.xlarge/PostgreSQL/Multi-AZ ($0.899/hr),
// ElastiCache cache.r6g.large/Redis ($0.206/hr), and the base ALB hourly rate
// ($0.0225/hr) — the exact SKUs/rateCodes the golden AWS bundle's own real sizing
// (PC-115) should match against.

import (
	"path/filepath"
	"strings"
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
	report := core.ComputeCost(ir, realPriceTable(), []string{"eu-west-1"}, prov)

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
	wantCache := 0.206 * core.HoursPerMonthAssumption * 3 // golden bundle's real num_cache_clusters = 3
	if cache.MonthlyAmount != wantCache {
		t.Errorf("ElastiCache MonthlyAmount = %v, want %v (hand-verified: $0.206/hr x 730 x 3 nodes)", cache.MonthlyAmount, wantCache)
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
	report := core.ComputeCost(ir, realPriceTable(), []string{"eu-west-1"}, prov)

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

	r1 := core.ComputeCost(ir, table, []string{"eu-west-1"}, prov)
	r2 := core.ComputeCost(ir, table, []string{"eu-west-1"}, prov)

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
	report := core.ComputeCost(ir, realPriceTable(), []string{"eu-west-1"}, prov)
	if len(report.Components) != 1 || report.Components[0].Decision != core.CostUnknown {
		t.Fatalf("got %+v, want a single cost_unknown component (no Sizing at all)", report.Components)
	}
	if report.Components[0].Reason == "" {
		t.Error("expected a real reason naming the missing sizing, got empty string")
	}
}

// ---- NAT gateway hourly price (ADR-006 amendment, 2026-10-02) ----
// Rows are REAL AmazonEC2 eu-west-1 NAT Gateway rows (offer 20260925174521), with the regional-NAT
// and provisioned-bandwidth siblings that must not be mistaken for the zonal hourly rate.

func natRows() []core.PriceRow {
	return []core.PriceRow{
		{Service: "AmazonEC2", Region: "eu-west-1", Unit: "Hrs", Price: 0.048, Currency: "USD", SKUAttributes: map[string]string{"usagetype": "EU-NatGateway-Hours", "operation": "NatGateway", "rate_description": "$0.048 per NAT Gateway Hour"}},
		{Service: "AmazonEC2", Region: "eu-west-1", Unit: "Hrs", Price: 0.048, Currency: "USD", SKUAttributes: map[string]string{"usagetype": "EU-RegionalNatGateway-Hours", "operation": "RegionalNatGateway"}},
		{Service: "AmazonEC2", Region: "eu-west-1", Unit: "Gbps-hrs", Price: 1.164, Currency: "USD", SKUAttributes: map[string]string{"usagetype": "EU-NatGateway-Prvd-Gbps", "operation": "NatGateway"}},
		{Service: "AmazonEC2", Region: "eu-west-1", Unit: "GB", Price: 0.048, Currency: "USD", SKUAttributes: map[string]string{"usagetype": "EU-NatGateway-Bytes", "operation": "NatGateway"}},
	}
}

func natComponents(t *testing.T, bundle string, rows []core.PriceRow) map[string]core.ComponentCost {
	t.Helper()
	awsReg, err := awsprovider.Load()
	if err != nil {
		t.Fatal(err)
	}
	result, err := ingest.Ingest(filepath.Join("..", "golden", bundle), awsReg, 1)
	if err != nil || result.IR == nil {
		t.Fatalf("Ingest %s: %v", bundle, err)
	}
	report := core.ComputeCost(result.IR, core.PriceTable{SnapshotID: "s", Rows: rows}, []string{"eu-west-1"}, core.NewProvenance(core.KindDerived, "test"))
	out := map[string]core.ComponentCost{}
	for _, c := range report.Components {
		if strings.HasPrefix(c.NodeID, "aws_nat_gateway.") {
			out[c.NodeID] = c
		}
	}
	return out
}

// Hand-verified: $0.048/hr x 730 h = $35.04 per NAT gateway per month. NAT-per-AZ costs three times
// one NAT — the resilience/cost trade-off the report can now show.
func TestComputeCost_NATGateways_PricedFromRealRows(t *testing.T) {
	nats := natComponents(t, "aws", natRows())
	if len(nats) != 3 {
		t.Fatalf("golden/aws has 3 NAT gateways (one per AZ); found %d: %v", len(nats), nats)
	}
	var total float64
	for id, c := range nats {
		if c.Decision != core.CostPriced || c.MonthlyAmount != 0.048*core.HoursPerMonthAssumption {
			t.Errorf("%s = %+v, want priced at 0.048 x 730 = $35.04", id, c)
		}
		if !strings.Contains(c.SKURateCode, "NAT Gateway Hour") {
			t.Errorf("%s: rate code %q should be the hourly row, not a sibling", id, c.SKURateCode)
		}
		total += c.MonthlyAmount
	}
	if total < 105.11 || total > 105.13 {
		t.Errorf("NAT-per-AZ total = %.2f, want 3 x 35.04 = 105.12", total)
	}
}

func TestComputeCost_NATGateways_UnpricedWithoutARow_IsUnknownNeverZero(t *testing.T) {
	// Only the siblings that must not be mistaken for the zonal hourly rate.
	rows := []core.PriceRow{natRows()[1], natRows()[2], natRows()[3]}
	for id, c := range natComponents(t, "aws", rows) {
		if c.Decision != core.CostUnknown || c.MonthlyAmount != 0 || !strings.Contains(c.Reason, "no NAT gateway hourly price row") {
			t.Errorf("%s = %+v, want cost_unknown naming the missing row (a regional-NAT or provisioned-bandwidth row must not stand in)", id, c)
		}
	}
	// A different region's row does not price an eu-west-1 NAT.
	other := natRows()[:1]
	other[0].Region = "us-east-1"
	for id, c := range natComponents(t, "aws", other) {
		if c.Decision != core.CostUnknown {
			t.Errorf("%s priced from another region's row: %+v", id, c)
		}
	}
}
