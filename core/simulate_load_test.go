package core_test

// PC-127's own backend prerequisite: /simulate must expose PC-126's own real
// utilization data (core.ComponentLoad) alongside FlowDetail, so a Simulate-mode UI
// can show before/after utilization for one injected fault without a second
// round-trip or reimplementing the load engine client-side.

import (
	"testing"

	"preflight/core"
)

func TestSimulate_Load_PopulatedWhenJourneysDeclared(t *testing.T) {
	ir := buildFlowTestIR(true, true)
	workload := core.Workload{
		Journeys: []core.DeclaredJourney{
			{ID: "j1", Name: "j1", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432, Criticality: "tier1", PeakRPS: floatPtr(10), SteadyRPS: floatPtr(5)},
		},
		Capacity: map[string]float64{"managed_database_rps": 100},
	}

	resp := core.Simulate(ir, workload, nil, syntheticProv())
	if len(resp.Load) == 0 {
		t.Fatal("Load is empty, want real per-component utilization for the declared journey")
	}

	found := false
	for _, l := range resp.Load {
		if l.NodeID == "db" {
			found = true
			if l.Capacity == nil || *l.Capacity != 100 {
				t.Errorf("db Capacity = %v, want 100 (from workload.Capacity[managed_database_rps])", l.Capacity)
			}
		}
	}
	if !found {
		t.Error("expected a Load entry for node \"db\"")
	}
}

func TestSimulate_Load_ReflectsFault_BeforeAfterComparison(t *testing.T) {
	ir := buildFlowTestIR(true, true)
	workload := core.Workload{
		Journeys: []core.DeclaredJourney{
			{ID: "j1", Name: "j1", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432, Criticality: "tier1", PeakRPS: floatPtr(10), SteadyRPS: floatPtr(5)},
		},
		Capacity: map[string]float64{"managed_database_rps": 100},
	}

	before := core.Simulate(ir, workload, nil, syntheticProv())
	loadBefore := loadFor(before.Load, "db")
	if loadBefore == nil || loadBefore.OfferedRPS <= 0 {
		t.Fatalf("before fault: expected db to carry real offered load, got %+v", loadBefore)
	}

	after := core.Simulate(ir, workload, []core.Fault{{Type: "node_loss", Target: "app"}}, syntheticProv())
	loadAfter := loadFor(after.Load, "db")
	// "app" is killed, so no traffic reaches "db" at all under this fault — db must
	// not appear carrying load it can no longer receive (ComputeComponentLoad's own
	// "only hops that actually flow" rule, reused verbatim here, not reimplemented).
	if loadAfter != nil && loadAfter.OfferedRPS > 0 {
		t.Errorf("after killing app: db still shows offered load %v, want none — app is the only source of traffic to db", loadAfter.OfferedRPS)
	}
}

func loadFor(loads []core.ComponentLoad, nodeID string) *core.ComponentLoad {
	for i := range loads {
		if loads[i].NodeID == nodeID {
			return &loads[i]
		}
	}
	return nil
}
