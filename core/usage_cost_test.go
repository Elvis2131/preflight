package core_test

// PC-132's own acceptance criteria, each cited below. GOLDEN-FIXTURE HONESTY NOTE,
// investigated before writing these tests: golden/aws's own real subnet
// availability_zone attributes are declared via a Terraform local value
// (`local.az_a`, golden/aws/network.tf) rather than a static literal — CLAUDE.md
// §15's own static-literal-only ingest scope does not resolve locals, so
// availability_zone comes back genuinely absent for every real golden subnet (checked
// directly: RawAttributes["availability_zone"] is nil for aws_subnet.public_a).
// Cross-AZ transfer is therefore hand-verified on a synthetic fixture (literal AZ
// strings), not golden — golden itself correctly reports not_assessable for it,
// which IS this file's own hand-verified expected golden result. Separately, golden's
// own real checkout journey is entirely INBOUND (internet -> ALB -> EKS -> RDS); NAT
// gateway data processing only ever applies to a private-subnet resource's OWN
// outbound egress to the internet, which no golden journey's own declared path
// exercises (checkout terminates at RDS; settlement's path is EKS -> SQS, an AWS API
// call, not an internet-sentinel hop) — so a real, correct, hand-verified golden
// result for NAT is "zero NAT entries," not a fabricated positive example. NAT
// pricing IS hand-verified against a real dollar figure on a synthetic fixture below.

import (
	"testing"

	"preflight/core"
)

// usageCostPriceTable holds rows shaped exactly like real AWS Bulk Price List rows
// (eu-west-1 offers, retrieved 2026-10-02; prices/units/tiers are the real ones,
// attribute names as pricingfetch emits them). The RegionalNatGateway row is a real
// decoy: same unit and price, different product — it must never be the one matched.
func usageCostPriceTable() core.PriceTable {
	egress := func(begin, end string, price float64) core.PriceRow {
		return core.PriceRow{Service: "AWSDataTransfer", Region: "eu-west-1", Unit: "GB", Price: price, Currency: "USD",
			SKUAttributes: map[string]string{"usagetype": "EU-DataTransfer-Out-Bytes", "transferType": "AWS Outbound", "begin_range": begin, "end_range": end}}
	}
	return core.PriceTable{
		SnapshotID: "test-usage-snapshot",
		Rows: []core.PriceRow{
			{Service: "AmazonEC2", Region: "eu-west-1", SKUAttributes: map[string]string{"usagetype": "EU-RegionalNatGateway-Bytes", "operation": "RegionalNatGateway"}, Unit: "GB", Price: 0.048, Currency: "USD"},
			{Service: "AmazonEC2", Region: "eu-west-1", SKUAttributes: map[string]string{"usagetype": "EU-NatGateway-Bytes", "operation": "NatGateway"}, Unit: "GB", Price: 0.048, Currency: "USD"},
			egress("153600", "Inf", 0.05),
			egress("0", "10240", 0.09),
			egress("10240", "51200", 0.085),
			egress("51200", "153600", 0.07),
			{Service: "AWSDataTransfer", Region: "eu-west-1", SKUAttributes: map[string]string{"usagetype": "EU-DataTransfer-Regional-Bytes", "transferType": "IntraRegion"}, Unit: "GB", Price: 0.01, Currency: "USD"},
			// Real eu-west-1 AWSELB rows (offer 20260911124544): the on-demand LCU rate and two decoys that
			// must NOT match — the Outposts rate ($0) and the reserved-capacity rate.
			{Service: "AWSELB", Region: "eu-west-1", SKUAttributes: map[string]string{"usagetype": "EU-LCUUsage", "operation": "LoadBalancing:Application"}, Unit: "LCU-Hrs", Price: 0.008, Currency: "USD"},
			{Service: "AWSELB", Region: "eu-west-1", SKUAttributes: map[string]string{"usagetype": "EU-Outposts-LCUUsage", "operation": "LoadBalancing:Application"}, Unit: "LCU-Hrs", Price: 0, Currency: "USD"},
			{Service: "AWSELB", Region: "eu-west-1", SKUAttributes: map[string]string{"usagetype": "EU-ReservedLCUUsage", "operation": "LoadBalancing:Application"}, Unit: "ReservedLCU-Hr", Price: 0.008, Currency: "USD"},
		},
	}
}

func TestComputeUsageBasedCost_MissingInput_NamesTheField(t *testing.T) {
	ir := buildFlowTestIR(true, true)
	cases := []struct {
		name string
		j    core.DeclaredJourney
		want string
	}{
		{"no steady_rps", core.DeclaredJourney{ID: "j1", Name: "j1", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432, Criticality: "tier1"}, "steady_rps"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			results := core.ComputeUsageBasedCost(ir, core.Workload{Journeys: []core.DeclaredJourney{tc.j}}, usageCostPriceTable(), nil)
			if len(results) != 1 {
				t.Fatalf("got %d entries, want exactly 1 not_assessable entry", len(results))
			}
			if results[0].Decision != core.CostUnknown {
				t.Fatalf("Decision = %q, want cost_unknown", results[0].Decision)
			}
			if !containsSubstr(results[0].Reason, tc.want) {
				t.Errorf("Reason = %q, want it to name %q", results[0].Reason, tc.want)
			}
		})
	}
}

func steadyRPS(v float64) *float64 { return &v }
func avgBytes(v float64) *float64  { return &v }

// buildInternetReachableIR is a minimal fixture for internet-originated hops —
// buildFlowTestIR's own "db" destination is deliberately internet-UNREACHABLE (its SG
// only permits tcp/5432 from a specific internal CIDR, per its own doc comment), so
// an internet-sentinel journey needs its own destination with a real permissive SG
// (tcp/443 from 0.0.0.0/0) and NACL.
func buildInternetReachableIR() *core.IR {
	prov := syntheticProv()
	subnet := rt("subnetPub", core.NodeTypeNetworkBoundary)
	subnet.RawAttributes = map[string]any{"cidr_block": "10.0.0.0/24"}
	sg := rt("sgPub", core.NodeTypeNetworkBoundary)
	sg.RawAttributes = map[string]any{"security_group_rules": []map[string]any{
		{"direction": "ingress", "protocol": "tcp", "from_port": 443, "to_port": 443, "cidr_blocks": []any{"0.0.0.0/0"}},
	}}
	nacl := rt("naclPub", core.NodeTypeNetworkBoundary)
	nacl.RawAttributes = map[string]any{"nacl_rules": []map[string]any{
		{"number": 100, "direction": "ingress", "protocol": "-1", "cidr": "0.0.0.0/0", "allow": true},
		{"number": 100, "direction": "egress", "protocol": "-1", "cidr": "0.0.0.0/0", "allow": true},
	}}
	dest := rtCapable("edge", core.NodeTypeLoadBalancer, core.CapabilityRequestSimulation)
	nodes := []core.Node{dest, subnet, sg, nacl}
	edges := []core.Edge{
		{ID: "e1", Type: core.EdgeTypeContainedIn, From: "edge", To: "subnetPub", Resolution: core.ResolutionKnown, Provenance: prov},
		{ID: "e2", Type: core.EdgeTypeDependsOn, From: "edge", To: "sgPub", Resolution: core.ResolutionKnown, Provenance: prov},
		{ID: "e3", Type: core.EdgeTypeDependsOn, From: "subnetPub", To: "naclPub", Resolution: core.ResolutionKnown, Provenance: prov},
	}
	return &core.IR{SchemaVersion: "1.3.0", VersionNumber: 1, VersionHash: "h", Nodes: nodes, Edges: edges}
}

func TestComputeUsageBasedCost_InternetEgress_RealPricedAmount(t *testing.T) {
	ir := buildInternetReachableIR()
	j := core.DeclaredJourney{
		ID: "j1", Name: "j1", Path: []string{core.JourneyInternetSentinel, "edge"}, Protocol: "tcp", Port: 443, Criticality: "tier1",
		SteadyRPS: steadyRPS(10), AvgRequestBytes: avgBytes(1000), AvgResponseBytes: avgBytes(4000),
	}
	results := core.ComputeUsageBasedCost(ir, core.Workload{Journeys: []core.DeclaredJourney{j}}, usageCostPriceTable(), nil)

	var egress *core.UsageBasedCostEntry
	for i := range results {
		if results[i].Kind == core.UsageCostInternetEgress {
			egress = &results[i]
		}
	}
	if egress == nil {
		t.Fatalf("expected an internet_egress entry, got %+v", results)
	}
	if egress.Decision != core.CostPriced {
		t.Fatalf("Decision = %q, want priced: %s", egress.Decision, egress.Reason)
	}
	// Hand-verified: only the RESPONSE leaves AWS on an internet-originated hop (inbound
	// transfer is free): 10 req/s * 4000 response bytes * (730*3600) s/month = ~97.9 GB
	// (bytes / 1024^3), all within the first tier.
	wantGB := 10.0 * 4000.0 * core.SecondsPerMonthAssumption / (1024 * 1024 * 1024)
	// ~122.4 GB sits wholly inside the first real eu-west-1 tier ($0.09, 0-10240 GB).
	wantAmount := wantGB * 0.09
	if diff := egress.MonthlyGB - wantGB; diff > 0.001 || diff < -0.001 {
		t.Errorf("MonthlyGB = %v, want %v", egress.MonthlyGB, wantGB)
	}
	if diff := egress.MonthlyAmount - wantAmount; diff > 0.01 || diff < -0.01 {
		t.Errorf("MonthlyAmount = %v, want %v", egress.MonthlyAmount, wantAmount)
	}
	if egress.Provenance.Kind != core.KindDerived {
		t.Errorf("entry Provenance.Kind = %q, want derived (it is the priced fact; the ASSUMED conversion is cited in its own Reason string)", egress.Provenance.Kind)
	}
}

func TestComputeUsageBasedCost_NoMatchingPriceRow_CostUnknown(t *testing.T) {
	ir := buildInternetReachableIR()
	j := core.DeclaredJourney{
		ID: "j1", Name: "j1", Path: []string{core.JourneyInternetSentinel, "edge"}, Protocol: "tcp", Port: 443, Criticality: "tier1",
		SteadyRPS: steadyRPS(10), AvgRequestBytes: avgBytes(1000), AvgResponseBytes: avgBytes(4000),
	}
	empty := core.PriceTable{SnapshotID: "empty"}
	results := core.ComputeUsageBasedCost(ir, core.Workload{Journeys: []core.DeclaredJourney{j}}, empty, nil)
	for _, r := range results {
		if r.Decision != core.CostUnknown {
			t.Errorf("Decision = %q, want cost_unknown (no price rows at all): %+v", r.Decision, r)
		}
	}
}

// TestComputeUsageBasedCost_CrossAZ_SyntheticFixture is the Card's own acceptance
// criterion, verbatim: "Cross-AZ transfer appears for multi-AZ designs and not for
// single-AZ, following structural flow — tested." Golden/aws cannot exercise this
// (see this file's own header note); a synthetic fixture with literal AZ strings can.
func TestComputeUsageBasedCost_CrossAZ_SyntheticFixture(t *testing.T) {
	multiAZ := buildFlowTestIR(true, true)
	setSubnetAZ(multiAZ, "subnetA", "eu-west-1a")
	setSubnetAZ(multiAZ, "subnetB", "eu-west-1b")

	singleAZ := buildFlowTestIR(true, true)
	setSubnetAZ(singleAZ, "subnetA", "eu-west-1a")
	setSubnetAZ(singleAZ, "subnetB", "eu-west-1a")

	j := core.DeclaredJourney{
		ID: "j1", Name: "j1", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432, Criticality: "tier1",
		SteadyRPS: steadyRPS(10), AvgRequestBytes: avgBytes(1000), AvgResponseBytes: avgBytes(4000),
	}

	multiResults := core.ComputeUsageBasedCost(multiAZ, core.Workload{Journeys: []core.DeclaredJourney{j}}, usageCostPriceTable(), nil)
	if !hasKind(multiResults, core.UsageCostCrossAZTransfer, core.CostPriced) {
		t.Errorf("multi-AZ fixture: expected a priced cross_az_transfer entry, got %+v", multiResults)
	}

	singleResults := core.ComputeUsageBasedCost(singleAZ, core.Workload{Journeys: []core.DeclaredJourney{j}}, usageCostPriceTable(), nil)
	if hasKind(singleResults, core.UsageCostCrossAZTransfer, core.CostPriced) {
		t.Errorf("single-AZ fixture: expected NO cross_az_transfer entry, got %+v", singleResults)
	}
}

func TestComputeUsageBasedCost_CrossAZ_UnresolvableAZ_NotAssessable(t *testing.T) {
	ir := buildFlowTestIR(true, true) // no availability_zone set on either subnet
	j := core.DeclaredJourney{
		ID: "j1", Name: "j1", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432, Criticality: "tier1",
		SteadyRPS: steadyRPS(10), AvgRequestBytes: avgBytes(1000), AvgResponseBytes: avgBytes(4000),
	}
	results := core.ComputeUsageBasedCost(ir, core.Workload{Journeys: []core.DeclaredJourney{j}}, usageCostPriceTable(), nil)
	found := false
	for _, r := range results {
		if r.Kind == core.UsageCostCrossAZTransfer {
			found = true
			if r.Decision != core.CostUnknown {
				t.Errorf("Decision = %q, want cost_unknown — neither subnet declares availability_zone", r.Decision)
			}
		}
	}
	if !found {
		t.Fatal("expected a cross_az_transfer entry (even if cost_unknown)")
	}
}

func TestComputeUsageBasedCost_LBDataProcessed(t *testing.T) {
	// buildInternetReachableIR's own "edge" node is already NodeTypeLoadBalancer.
	ir := buildInternetReachableIR()
	j := core.DeclaredJourney{
		ID: "j1", Name: "j1", Path: []string{core.JourneyInternetSentinel, "edge"}, Protocol: "tcp", Port: 443, Criticality: "tier1",
		SteadyRPS: steadyRPS(10), AvgRequestBytes: avgBytes(1000), AvgResponseBytes: avgBytes(4000),
	}
	results := core.ComputeUsageBasedCost(ir, core.Workload{Journeys: []core.DeclaredJourney{j}}, usageCostPriceTable(), nil)
	if !hasKind(results, core.UsageCostLBDataProcessed, core.CostPriced) {
		t.Errorf("expected a priced lb_data_processed entry, got %+v", results)
	}
}

// TestGoldenUsageCost_Checkout_BaselineDoesNotFlow_HonestlyEmpty is the golden-bundle
// half of this ticket's own acceptance criterion, hand-verified against what
// golden/aws actually supports today — investigated directly before writing this
// test: golden/aws's own checkout journey does not structurally flow AT ALL
// (core.ComputeJourneyFlow blocks at its very first hop, internet -> aws_lb.payments,
// citing golden/aws's own real, pre-existing, already-documented gap: zero NACL
// resources exist anywhere in that bundle, so nacl_check fails universally —
// PC-125/130's own documented limitation, not something PC-132 introduces). Since
// usage-based cost is only ever computed over hops a journey's own structural flow
// says actually carry traffic, a journey with zero flowing hops honestly produces
// zero usage-cost entries — the correct, hand-verified golden result, not a
// fabricated positive example. avg_request_bytes/avg_response_bytes were added to
// golden/workload.yaml's own checkout journey by this ticket specifically so this
// gap is visible in the SAME way the missing-inputs case is (rather than every
// golden journey looking identical — "missing input" vs "declared input, but the
// underlying baseline itself does not flow" are two different honest reasons for the
// same zero-entries outcome, and this test pins down which one golden/aws
// demonstrates).
func TestGoldenUsageCost_Checkout_HandVerified(t *testing.T) {
	ir := realGoldenIR(t)
	workload := loadGoldenWorkload(t)

	// PC-152: with each hop's real port, the golden checkout journey flows end to end
	// (internet -> ALB -> EKS -> RDS), so every hop carries traffic and is costed.
	// Hand-verified: 500 rps x (2048 request + 8192 response bytes) x 2,628,000 s / 1024^3
	// = 12,531.28 GB/month through each hop. Internet egress bills ONLY the 8192-byte
	// responses (inbound is free): 10,025.02 GB, all in the first tier ($0.09/GB) =
	// $902.25. The ALB's LCU processed-bytes lower bound uses the full volume:
	// 12,531.28 LCU-hours x $0.008 = $100.25 on each hop the ALB touches. Cross-AZ is
	// cost_unknown because golden's subnet AZs come from unresolved Terraform locals.
	type key struct {
		from, to string
		kind     core.UsageCostKind
	}
	got := map[key]core.UsageBasedCostEntry{}
	for _, r := range core.ComputeUsageBasedCost(ir, workload, usageCostPriceTable(), nil) {
		if r.JourneyID == "checkout" {
			got[key{r.HopFrom, r.HopTo, r.Kind}] = r
		}
	}
	want := []struct {
		k      key
		priced bool
		amount float64
	}{
		{key{core.JourneyInternetSentinel, "aws_lb.payments", core.UsageCostInternetEgress}, true, 10025.0244140625 * 0.09},
		{key{core.JourneyInternetSentinel, "aws_lb.payments", core.UsageCostLBDataProcessed}, true, 12531.280517578125 * 0.008},
		{key{"aws_lb.payments", "aws_eks_cluster.payments", core.UsageCostLBDataProcessed}, true, 12531.280517578125 * 0.008},
		{key{"aws_lb.payments", "aws_eks_cluster.payments", core.UsageCostCrossAZTransfer}, false, 0},
		{key{"aws_eks_cluster.payments", "aws_db_instance.payments", core.UsageCostCrossAZTransfer}, false, 0},
	}
	if len(got) != len(want) {
		t.Errorf("got %d checkout usage entries, want %d: %+v", len(got), len(want), got)
	}
	for _, w := range want {
		r, ok := got[w.k]
		if !ok {
			t.Errorf("missing %+v", w.k)
			continue
		}
		if w.priced {
			if r.Decision != core.CostPriced || r.MonthlyAmount < w.amount-0.01 || r.MonthlyAmount > w.amount+0.01 {
				t.Errorf("%+v = %v $%.2f, want priced $%.2f", w.k, r.Decision, r.MonthlyAmount, w.amount)
			}
		} else if r.Decision != core.CostUnknown || r.MonthlyAmount != 0 {
			t.Errorf("%+v = %v $%.2f, want cost_unknown (no resolvable availability zone)", w.k, r.Decision, r.MonthlyAmount)
		}
	}
}

func setSubnetAZ(ir *core.IR, subnetID, az string) {
	for i := range ir.Nodes {
		if ir.Nodes[i].ID == subnetID {
			if ir.Nodes[i].RawAttributes == nil {
				ir.Nodes[i].RawAttributes = map[string]any{}
			}
			ir.Nodes[i].RawAttributes["availability_zone"] = az
			return
		}
	}
	panic("subnet not found: " + subnetID)
}

func hasKind(results []core.UsageBasedCostEntry, kind core.UsageCostKind, decision core.CostDecision) bool {
	for _, r := range results {
		if r.Kind == kind && r.Decision == decision {
			return true
		}
	}
	return false
}

func containsSubstr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestMatchers_RealRowShapes_NegativeControls pins the defects found by checking the
// matchers against real Price List data: the old substring match let
// RegionalNatGateway-Bytes satisfy NatGateway-Bytes, and EC2 rows could never carry
// egress. Each case fails if the corresponding guard is removed.
func TestUsageCost_RealRowShapes_NegativeControls(t *testing.T) {
	ir := buildInternetReachableIR()
	j := core.DeclaredJourney{
		ID: "j1", Name: "j1", Path: []string{core.JourneyInternetSentinel, "edge"}, Protocol: "tcp", Port: 443, Criticality: "tier1",
		SteadyRPS: steadyRPS(10), AvgRequestBytes: avgBytes(1000), AvgResponseBytes: avgBytes(4000),
	}
	run := func(rows ...core.PriceRow) []core.UsageBasedCostEntry {
		return core.ComputeUsageBasedCost(ir, core.Workload{Journeys: []core.DeclaredJourney{j}}, core.PriceTable{SnapshotID: "s", Rows: rows}, nil)
	}
	find := func(rs []core.UsageBasedCostEntry, k core.UsageCostKind) core.UsageBasedCostEntry {
		for _, r := range rs {
			if r.Kind == k {
				return r
			}
		}
		t.Fatalf("no %s entry in %+v", k, rs)
		return core.UsageBasedCostEntry{}
	}

	// Egress rows without tier bounds are refused, not priced at one arbitrary tier.
	noTiers := run(core.PriceRow{Region: "eu-west-1", Unit: "GB", Price: 0.09, Currency: "USD",
		SKUAttributes: map[string]string{"usagetype": "EU-DataTransfer-Out-Bytes", "transferType": "AWS Outbound"}})
	if e := find(noTiers, core.UsageCostInternetEgress); e.Decision != core.CostUnknown {
		t.Errorf("egress with no tier bounds: %+v, want cost_unknown", e)
	}
	// Wrong unit is skipped, never multiplied blindly.
	badUnit := run(core.PriceRow{Region: "eu-west-1", Unit: "Hrs", Price: 0.09, Currency: "USD",
		SKUAttributes: map[string]string{"usagetype": "EU-DataTransfer-Out-Bytes", "transferType": "AWS Outbound", "begin_range": "0", "end_range": "Inf"}})
	if e := find(badUnit, core.UsageCostInternetEgress); e.Decision != core.CostUnknown {
		t.Errorf("egress with unit Hrs: %+v, want cost_unknown", e)
	}
	// The same usagetype from two regions with nothing to choose between them.
	two := run(
		core.PriceRow{Region: "eu-west-1", Unit: "GB", Price: 0.09, Currency: "USD", SKUAttributes: map[string]string{"usagetype": "EU-DataTransfer-Out-Bytes", "transferType": "AWS Outbound", "begin_range": "0", "end_range": "Inf"}},
		core.PriceRow{Region: "us-east-1", Unit: "GB", Price: 0.09, Currency: "USD", SKUAttributes: map[string]string{"usagetype": "DataTransfer-Out-Bytes", "transferType": "AWS Outbound", "begin_range": "0", "end_range": "Inf"}},
	)
	if e := find(two, core.UsageCostInternetEgress); e.Decision != core.CostUnknown {
		t.Errorf("rows from two regions: %+v, want cost_unknown", e)
	}
	// Row order must not change the answer.
	tbl := usageCostPriceTable()
	rev := core.PriceTable{SnapshotID: tbl.SnapshotID}
	for i := len(tbl.Rows) - 1; i >= 0; i-- {
		rev.Rows = append(rev.Rows, tbl.Rows[i])
	}
	a := core.ComputeUsageBasedCost(ir, core.Workload{Journeys: []core.DeclaredJourney{j}}, tbl, nil)
	b := core.ComputeUsageBasedCost(ir, core.Workload{Journeys: []core.DeclaredJourney{j}}, rev, nil)
	if len(a) != len(b) {
		t.Fatalf("entry counts differ by row order: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].MonthlyAmount != b[i].MonthlyAmount || a[i].Decision != b[i].Decision {
			t.Errorf("entry %d differs by row order: %+v vs %+v", i, a[i], b[i])
		}
	}
}

// A volume that spans tiers: 6000 req/s x 4000 response bytes x 2,628,000 s = ~58,740
// GB/month leaving AWS, so 10240 GB at $0.09 + 40960 GB at $0.085 + the rest at $0.07
// (real eu-west-1 tiers).
func TestComputeUsageBasedCost_InternetEgress_SpansTiers(t *testing.T) {
	ir := buildInternetReachableIR()
	j := core.DeclaredJourney{
		ID: "j1", Name: "j1", Path: []string{core.JourneyInternetSentinel, "edge"}, Protocol: "tcp", Port: 443, Criticality: "tier1",
		SteadyRPS: steadyRPS(6000), AvgRequestBytes: avgBytes(1000), AvgResponseBytes: avgBytes(4000),
	}
	results := core.ComputeUsageBasedCost(ir, core.Workload{Journeys: []core.DeclaredJourney{j}}, usageCostPriceTable(), nil)
	for _, r := range results {
		if r.Kind != core.UsageCostInternetEgress {
			continue
		}
		gb := 6000.0 * 4000.0 * core.SecondsPerMonthAssumption / (1024 * 1024 * 1024)
		want := 10240*0.09 + 40960*0.085 + (gb-51200)*0.07
		if diff := r.MonthlyAmount - want; diff > 0.01 || diff < -0.01 {
			t.Errorf("MonthlyAmount = %v, want %v", r.MonthlyAmount, want)
		}
		return
	}
	t.Fatal("no internet_egress entry")
}
