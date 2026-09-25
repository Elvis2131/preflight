package core_test

// PC-126: ComputeComponentLoad/RankBottlenecks/BuildLoadFindings' own tests, against
// the same hand-built synthetic architecture core/journey_flow_test.go established
// (real per-property behavior needs a scenario built to exercise it).

import (
	"testing"

	"preflight/core"
)

func TestComputeComponentLoad_HandVerified_SingleJourney(t *testing.T) {
	ir := buildFlowTestIR(true, true)
	workload := core.Workload{
		Journeys: []core.DeclaredJourney{
			{ID: "j1", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432, PeakRPS: floatPtr(100)},
		},
		Capacity: map[string]float64{
			core.JourneyCapacityKey(core.NodeTypeCompute):         200,
			core.JourneyCapacityKey(core.NodeTypeManagedDatabase): 150,
		},
	}
	loads := core.ComputeComponentLoad(ir, workload, nil)

	byID := map[string]core.ComponentLoad{}
	for _, l := range loads {
		byID[l.NodeID] = l
	}

	app, ok := byID["app"]
	if !ok || app.OfferedRPS != 100 {
		t.Fatalf("app: got %+v, want OfferedRPS=100", app)
	}
	if app.Utilization == nil || *app.Utilization != 0.5 {
		t.Errorf("app: Utilization = %v, want 0.5 (100/200, hand-verified)", app.Utilization)
	}

	db, ok := byID["db"]
	if !ok || db.OfferedRPS != 100 {
		t.Fatalf("db: got %+v, want OfferedRPS=100", db)
	}
	wantUtil := 100.0 / 150.0
	if db.Utilization == nil || *db.Utilization != wantUtil {
		t.Errorf("db: Utilization = %v, want %v (100/150, hand-verified)", db.Utilization, wantUtil)
	}
}

func TestComputeComponentLoad_SumsAcrossMultipleJourneys(t *testing.T) {
	ir := buildFlowTestIR(true, true)
	workload := core.Workload{
		Journeys: []core.DeclaredJourney{
			{ID: "j1", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432, PeakRPS: floatPtr(100)},
			{ID: "j2", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432, PeakRPS: floatPtr(50)},
		},
		Capacity: map[string]float64{
			core.JourneyCapacityKey(core.NodeTypeCompute):         500,
			core.JourneyCapacityKey(core.NodeTypeManagedDatabase): 500,
		},
	}
	loads := core.ComputeComponentLoad(ir, workload, nil)
	for _, l := range loads {
		if l.NodeID == "db" && l.OfferedRPS != 150 {
			t.Errorf("db: OfferedRPS = %v, want 150 (100+50, hand-verified sum across two journeys)", l.OfferedRPS)
		}
	}
}

func TestComputeComponentLoad_MissingCapacity_NotAssessable(t *testing.T) {
	ir := buildFlowTestIR(true, true)
	workload := core.Workload{
		Journeys: []core.DeclaredJourney{
			{ID: "j1", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432, PeakRPS: floatPtr(100)},
		},
		// No declared capacity at all.
	}
	loads := core.ComputeComponentLoad(ir, workload, nil)
	for _, l := range loads {
		if l.Utilization != nil {
			t.Errorf("%s: Utilization = %v, want nil (no declared capacity)", l.NodeID, l.Utilization)
		}
		if l.NotAssessableReason == "" {
			t.Errorf("%s: NotAssessableReason is empty, want a reason naming the missing capacity key", l.NodeID)
		}
	}
}

func TestComputeComponentLoad_JourneyMissingPeakRPS_ContributesNoLoad(t *testing.T) {
	ir := buildFlowTestIR(true, true)
	workload := core.Workload{
		Journeys: []core.DeclaredJourney{
			{ID: "j1", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432}, // no PeakRPS
		},
		Capacity: map[string]float64{
			core.JourneyCapacityKey(core.NodeTypeCompute):         500,
			core.JourneyCapacityKey(core.NodeTypeManagedDatabase): 500,
		},
	}
	loads := core.ComputeComponentLoad(ir, workload, nil)
	if len(loads) != 0 {
		t.Fatalf("got %+v, want no components at all — a journey with no declared peak_rps must never contribute a guessed load", loads)
	}
}

func TestComputeComponentLoad_BlockedHop_ContributesNoLoadPastTheBlock(t *testing.T) {
	ir := buildFlowTestIR(false, true) // SG denies app->db
	workload := core.Workload{
		Journeys: []core.DeclaredJourney{
			{ID: "j1", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432, PeakRPS: floatPtr(100)},
		},
		Capacity: map[string]float64{
			core.JourneyCapacityKey(core.NodeTypeCompute):         500,
			core.JourneyCapacityKey(core.NodeTypeManagedDatabase): 500,
		},
	}
	loads := core.ComputeComponentLoad(ir, workload, nil)
	for _, l := range loads {
		if l.NodeID == "db" {
			t.Fatalf("db must not receive any load — the only hop reaching it is blocked by SG, got %+v", l)
		}
	}
}

// TestComputeComponentLoad_FaultRecompute_HandVerified is PC-126's own acceptance
// criterion: "Under AZ loss, recomputed utilisation matches a hand-worked expected
// result."
func TestComputeComponentLoad_FaultRecompute_HandVerified(t *testing.T) {
	ir := buildFlowTestIR(true, true)
	workload := core.Workload{
		Journeys: []core.DeclaredJourney{
			{ID: "j1", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432, PeakRPS: floatPtr(100)},
		},
		Capacity: map[string]float64{
			core.JourneyCapacityKey(core.NodeTypeCompute):         500,
			core.JourneyCapacityKey(core.NodeTypeManagedDatabase): 500,
		},
	}

	before := core.ComputeComponentLoad(ir, workload, nil)
	var dbBefore *core.ComponentLoad
	for i := range before {
		if before[i].NodeID == "db" {
			dbBefore = &before[i]
		}
	}
	if dbBefore == nil || dbBefore.OfferedRPS != 100 {
		t.Fatalf("before the fault: got %+v, want db offered 100", dbBefore)
	}

	// db's own subnet is killed — the journey can no longer reach it at all.
	after := core.ComputeComponentLoad(ir, workload, map[string]bool{"db": true})
	for _, l := range after {
		if l.NodeID == "db" {
			t.Fatalf("db is in the killed set — it must not appear in the load report at all (a dead component has no utilisation), got %+v", l)
		}
	}
}

func TestRankBottlenecks_SortedDescendingAndOnlyAssessed(t *testing.T) {
	loads := []core.ComponentLoad{
		{NodeID: "low", Utilization: floatPtr(0.2)},
		{NodeID: "high", Utilization: floatPtr(1.5)},
		{NodeID: "unassessed"}, // nil Utilization
		{NodeID: "mid", Utilization: floatPtr(0.8)},
	}
	ranked := core.RankBottlenecks(loads)
	if len(ranked) != 3 {
		t.Fatalf("got %d ranked components, want 3 (unassessed excluded)", len(ranked))
	}
	if ranked[0].NodeID != "high" || ranked[1].NodeID != "mid" || ranked[2].NodeID != "low" {
		t.Fatalf("got order %v, want [high, mid, low] (descending utilisation)", []string{ranked[0].NodeID, ranked[1].NodeID, ranked[2].NodeID})
	}
}

func TestBuildLoadFindings_OverCapacity_Unsatisfied(t *testing.T) {
	prov := core.NewProvenance(core.KindDerived, "test")
	loads := []core.ComponentLoad{
		{NodeID: "db", OfferedRPS: 200, Capacity: floatPtr(100), Utilization: floatPtr(2.0)},
	}
	findings := core.BuildLoadFindings(loads, prov)
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1", len(findings))
	}
	status, _ := findings[0].Outcome.Value.(string)
	if status != "unsatisfied" {
		t.Errorf("status = %q, want unsatisfied (200 rps offered against 100 declared capacity)", status)
	}
}

func TestBuildLoadFindings_WithinCapacity_Satisfied(t *testing.T) {
	prov := core.NewProvenance(core.KindDerived, "test")
	loads := []core.ComponentLoad{
		{NodeID: "db", OfferedRPS: 50, Capacity: floatPtr(100), Utilization: floatPtr(0.5)},
	}
	findings := core.BuildLoadFindings(loads, prov)
	status, _ := findings[0].Outcome.Value.(string)
	if status != "satisfied" {
		t.Errorf("status = %q, want satisfied", status)
	}
}

func TestBuildLoadFindings_NoCapacity_NotAssessable(t *testing.T) {
	prov := core.NewProvenance(core.KindDerived, "test")
	loads := []core.ComponentLoad{
		{NodeID: "db", OfferedRPS: 50, NotAssessableReason: "no declared capacity for node type \"managed_database\""},
	}
	findings := core.BuildLoadFindings(loads, prov)
	if findings[0].Outcome.State != core.AssessmentStateNotAssessable {
		t.Errorf("State = %v, want not_assessable", findings[0].Outcome.State)
	}
}
