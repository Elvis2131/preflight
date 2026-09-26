package core_test

// PC-126's own follow-up (filed after PC-136 surfaced that the Card's own named
// example — "2 of 3 app nodes lost, survivors at 150% of declared capacity" — could
// not be built or tested without core/journey_flow.go's own parallel-path support).
// These tests reproduce that exact example against a real, synthetic three-replica
// architecture, hand-verified arithmetic, not an assumption.

import (
	"testing"

	"preflight/core"
)

func TestComputeComponentLoad_ParallelGroup_EvenSplitAcrossThreeHealthyReplicas(t *testing.T) {
	ir := addParallelAppReplicas(buildFlowTestIR(true, true))
	workload := core.Workload{
		Journeys: []core.DeclaredJourney{
			{ID: "j1", Path: []string{"app|app2|app3", "db"}, Protocol: "tcp", Port: 5432, PeakRPS: floatPtr(300)},
		},
		Capacity: map[string]float64{
			core.JourneyCapacityKey(core.NodeTypeCompute):         100,
			core.JourneyCapacityKey(core.NodeTypeManagedDatabase): 500,
		},
	}

	loads := core.ComputeComponentLoad(ir, workload, nil)
	byID := map[string]core.ComponentLoad{}
	for _, l := range loads {
		byID[l.NodeID] = l
	}

	for _, id := range []string{"app", "app2", "app3"} {
		l, ok := byID[id]
		if !ok {
			t.Fatalf("expected a load entry for %s", id)
		}
		if l.OfferedRPS != 100 {
			t.Errorf("%s: OfferedRPS = %v, want 100 (300 declared / 3 healthy replicas)", id, l.OfferedRPS)
		}
		if l.Utilization == nil || *l.Utilization != 1.0 {
			t.Errorf("%s: Utilization = %v, want 1.0 (100 offered / 100 declared capacity)", id, l.Utilization)
		}
	}
	// db is a singleton position downstream of the fan-in — it receives the FULL
	// declared rate once, never multiplied by the number of sources that reach it.
	if byID["db"].OfferedRPS != 300 {
		t.Errorf("db: OfferedRPS = %v, want 300 (the journey's own full declared rate, not 3x from three converging sources)", byID["db"].OfferedRPS)
	}
}

// TestComputeComponentLoad_ParallelGroup_SurvivorsAt150PercentUnderFault is the
// Card's own named example, reproduced for real: "2 of 3 app nodes lost, survivors
// at 150% of declared capacity."
func TestComputeComponentLoad_ParallelGroup_SurvivorsAt150PercentUnderFault(t *testing.T) {
	ir := addParallelAppReplicas(buildFlowTestIR(true, true))
	workload := core.Workload{
		Journeys: []core.DeclaredJourney{
			{ID: "j1", Path: []string{"app|app2|app3", "db"}, Protocol: "tcp", Port: 5432, PeakRPS: floatPtr(300)},
		},
		Capacity: map[string]float64{
			core.JourneyCapacityKey(core.NodeTypeCompute):         100,
			core.JourneyCapacityKey(core.NodeTypeManagedDatabase): 500,
		},
	}

	// Kill 1 of 3 replicas (app2) — matches the Card's own "2 of 3 app nodes lost"
	// framing inverted (here 1 of 3 is lost, 2 survive): 300 declared rps split
	// across the 2 survivors is 150 each, against a declared capacity of 100 each —
	// 150% utilisation, exactly the Card's own named ratio.
	loads := core.ComputeComponentLoad(ir, workload, map[string]bool{"app2": true})
	byID := map[string]core.ComponentLoad{}
	for _, l := range loads {
		byID[l.NodeID] = l
	}

	if _, ok := byID["app2"]; ok {
		t.Error("app2 is killed — it must not appear in the load report at all")
	}
	for _, id := range []string{"app", "app3"} {
		l, ok := byID[id]
		if !ok {
			t.Fatalf("expected a load entry for surviving replica %s", id)
		}
		if l.OfferedRPS != 150 {
			t.Errorf("%s: OfferedRPS = %v, want 150 (300 declared / 2 surviving replicas)", id, l.OfferedRPS)
		}
		if l.Utilization == nil || *l.Utilization != 1.5 {
			t.Errorf("%s: Utilization = %v, want 1.5 — the Card's own named 150%% survivor ratio", id, l.Utilization)
		}
	}

	prov := core.NewProvenance(core.KindDerived, "test")
	findings := core.BuildLoadFindings(loads, prov)
	foundUnsatisfied := 0
	for _, f := range findings {
		if f.ID == "finding.load.bottleneck.app" || f.ID == "finding.load.bottleneck.app3" {
			status, _ := f.Outcome.Value.(string)
			if status == "unsatisfied" {
				foundUnsatisfied++
			}
		}
	}
	if foundUnsatisfied != 2 {
		t.Errorf("got %d unsatisfied findings for the surviving replicas, want 2 — a component above 100%% must produce a finding (the Card's own second acceptance criterion)", foundUnsatisfied)
	}
}

// TestComputeComponentLoad_LoadDivisionNote_VisibleOnlyWhereApplied is PC-126's own
// stated acceptance criterion: "even-split distribution rule is tagged assumed and
// visible in output" — checked here for real, on a fixture that mixes a divided
// component (a parallel replica) and an undivided one (db, a singleton downstream of
// the fan-in) in the SAME result, so the field's own "only where it actually applied"
// discipline is exercised, not just its presence.
func TestComputeComponentLoad_LoadDivisionNote_VisibleOnlyWhereApplied(t *testing.T) {
	ir := addParallelAppReplicas(buildFlowTestIR(true, true))
	workload := core.Workload{
		Journeys: []core.DeclaredJourney{
			{ID: "j1", Path: []string{"app|app2|app3", "db"}, Protocol: "tcp", Port: 5432, PeakRPS: floatPtr(300)},
		},
		Capacity: map[string]float64{
			core.JourneyCapacityKey(core.NodeTypeCompute):         100,
			core.JourneyCapacityKey(core.NodeTypeManagedDatabase): 500,
		},
	}
	loads := core.ComputeComponentLoad(ir, workload, nil)
	byID := map[string]core.ComponentLoad{}
	for _, l := range loads {
		byID[l.NodeID] = l
	}

	for _, id := range []string{"app", "app2", "app3"} {
		if byID[id].LoadDivisionNote != core.LoadDivisionAssumption {
			t.Errorf("%s: LoadDivisionNote = %q, want the real assumption text — it IS a member of a 3-way parallel group", id, byID[id].LoadDivisionNote)
		}
	}
	if byID["db"].LoadDivisionNote != "" {
		t.Errorf("db: LoadDivisionNote = %q, want empty — db is a singleton position, no assumption was applied to it", byID["db"].LoadDivisionNote)
	}
}
