package core_test

// PC-110: per-component region resolution and region-aware pricing. The rule, from the
// ticket's decision comment: the component's own region first; else the workload's region
// ONLY if it declares exactly one; else unresolved — the component is unpriced naming
// "region" until the architect chooses. The first of several is never picked.

import (
	"strings"
	"testing"

	"preflight/core"
)

func sp(s string) *string { return &s }

func TestResolveComponentRegion_ThreeStepRule(t *testing.T) {
	cases := []struct {
		name       string
		sizing     *core.Sizing
		workload   []string
		wantRegion string
		wantSource core.RegionSource
		wantReason string // substring when unresolved
	}{
		{"component's own region wins over a single workload region", &core.Sizing{Region: sp("us-east-1")}, []string{"eu-west-1"}, "us-east-1", core.RegionFromComponent, ""},
		{"component's own region wins when the workload declares several", &core.Sizing{Region: sp("us-east-1")}, []string{"eu-west-1", "eu-central-1"}, "us-east-1", core.RegionFromComponent, ""},
		{"no component region: exactly one workload region is used", &core.Sizing{}, []string{"eu-west-1"}, "eu-west-1", core.RegionFromWorkload, ""},
		{"no sizing at all: exactly one workload region is used", nil, []string{"eu-west-1"}, "eu-west-1", core.RegionFromWorkload, ""},
		{"a blank component region is not a region", &core.Sizing{Region: sp("")}, []string{"eu-west-1"}, "eu-west-1", core.RegionFromWorkload, ""},
		{"several workload regions and no component region: unresolved, first never assumed", &core.Sizing{}, []string{"eu-west-1", "eu-central-1"}, "", "", "never assumed"},
		{"no workload region and no component region: unresolved", nil, nil, "", "", "region is missing"},
	}
	for _, c := range cases {
		region, source, reason := core.ResolveComponentRegion(c.sizing, c.workload)
		if region != c.wantRegion || source != c.wantSource {
			t.Errorf("%s: got (%q, %q), want (%q, %q)", c.name, region, source, c.wantRegion, c.wantSource)
		}
		if c.wantRegion == "" && !strings.Contains(reason, c.wantReason) {
			t.Errorf("%s: reason %q must contain %q", c.name, reason, c.wantReason)
		}
		if c.wantRegion != "" && reason != "" {
			t.Errorf("%s: a resolved region must carry no reason, got %q", c.name, reason)
		}
	}
	// Explicit: the first of several is NEVER what comes back.
	if r, _, _ := core.ResolveComponentRegion(nil, []string{"eu-west-1", "eu-central-1"}); r != "" {
		t.Errorf("picked %q from several declared regions — that is the silent default the cost rules forbid", r)
	}
}

func lbIR(sizing *core.Sizing) *core.IR {
	n := rt("lb", core.NodeTypeLoadBalancer)
	n.Sizing = sizing
	return &core.IR{SchemaVersion: "1.4.0", VersionNumber: 1, VersionHash: "h", Nodes: []core.Node{n}}
}

func albRow(region string, price float64) core.PriceRow {
	return core.PriceRow{
		Service: "AWSELB", Region: region, Unit: "Hrs", Price: price, Currency: "USD",
		SKUAttributes: map[string]string{"usagetype": "LoadBalancerUsage", "operation": "LoadBalancing:Application"},
	}
}

func TestComputeCost_PricesInTheResolvedRegionAndRecordsWhichStepSuppliedIt(t *testing.T) {
	prov := core.NewProvenance(core.KindDerived, "test")
	table := core.PriceTable{SnapshotID: "s", Rows: []core.PriceRow{albRow("eu-west-1", 0.0225), albRow("us-east-1", 0.0200)}}
	app := &core.Sizing{LoadBalancerType: sp("application")}

	// workload step: exactly one region declared
	c := core.ComputeCost(lbIR(app), table, []string{"us-east-1"}, prov).Components[0]
	if c.Decision != core.CostPriced || c.Region != "us-east-1" || c.RegionSource != core.RegionFromWorkload {
		t.Fatalf("workload step: %+v", c)
	}
	if want := 0.0200 * core.HoursPerMonthAssumption; c.MonthlyAmount != want {
		t.Errorf("must price against the us-east-1 row ($0.0200/h), got %v want %v", c.MonthlyAmount, want)
	}

	// component step: its own region beats several workload regions
	withRegion := &core.Sizing{LoadBalancerType: sp("application"), Region: sp("eu-west-1")}
	c = core.ComputeCost(lbIR(withRegion), table, []string{"us-east-1", "eu-central-1"}, prov).Components[0]
	if c.Decision != core.CostPriced || c.Region != "eu-west-1" || c.RegionSource != core.RegionFromComponent {
		t.Fatalf("component step: %+v", c)
	}
	if want := 0.0225 * core.HoursPerMonthAssumption; c.MonthlyAmount != want {
		t.Errorf("must price against the eu-west-1 row ($0.0225/h), got %v want %v", c.MonthlyAmount, want)
	}
}

func TestComputeCost_UnresolvedRegionIsCostUnknownNamingRegion_NeverTheFirst(t *testing.T) {
	prov := core.NewProvenance(core.KindDerived, "test")
	table := core.PriceTable{SnapshotID: "s", Rows: []core.PriceRow{albRow("eu-west-1", 0.0225), albRow("eu-central-1", 0.0250)}}
	rep := core.ComputeCost(lbIR(&core.Sizing{LoadBalancerType: sp("application")}), table, []string{"eu-west-1", "eu-central-1"}, prov)
	c := rep.Components[0]
	if c.Decision != core.CostUnknown || c.MonthlyAmount != 0 || !strings.Contains(c.Reason, "region") {
		t.Fatalf("want cost_unknown naming region, got %+v", c)
	}
	if c.Region != "" || c.RegionSource != "" {
		t.Errorf("no region was resolved, so none may be recorded: %+v", c)
	}
	if rep.PricedTotal != 0 || rep.UnpricedCount != 1 {
		t.Errorf("an unpriced component must be counted, never silently priced: %+v", rep)
	}
}

func TestComputeCost_ResolvedRegionWithNoRowForIt_IsCostUnknown(t *testing.T) {
	prov := core.NewProvenance(core.KindDerived, "test")
	table := core.PriceTable{SnapshotID: "s", Rows: []core.PriceRow{albRow("us-east-1", 0.0200)}}
	c := core.ComputeCost(lbIR(&core.Sizing{LoadBalancerType: sp("application"), Region: sp("eu-west-1")}), table, nil, prov).Components[0]
	if c.Decision != core.CostUnknown || !strings.Contains(c.Reason, "eu-west-1") {
		t.Fatalf("a snapshot with no eu-west-1 price must leave the component unpriced, naming the region: %+v", c)
	}
	if c.Region != "eu-west-1" {
		t.Errorf("the region that WAS resolved is still recorded: %+v", c)
	}
}

// A row that records no region cannot be filtered, so a resolved region does not exclude it
// (hand-built fixture snapshots predate region scoping); a row that records ANOTHER region is.
func TestComputeCost_RowsWithoutARegionAreNotExcluded(t *testing.T) {
	prov := core.NewProvenance(core.KindDerived, "test")
	table := core.PriceTable{SnapshotID: "s", Rows: []core.PriceRow{albRow("", 0.0225)}}
	c := core.ComputeCost(lbIR(&core.Sizing{LoadBalancerType: sp("application")}), table, []string{"eu-west-1"}, prov).Components[0]
	if c.Decision != core.CostPriced {
		t.Fatalf("a region-less row must still price a resolved component, got %+v", c)
	}
}
