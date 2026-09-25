package core_test

// PC-124: EvaluateJourneyLoadReadiness's own acceptance criterion, verbatim: "Missing
// rps or capacity produce not_assessable load results naming the missing field."

import (
	"strings"
	"testing"

	"preflight/core"
)

func floatPtr(f float64) *float64 { return &f }

func testIR() *core.IR {
	prov := core.NewProvenance(core.KindDerived, "test")
	return &core.IR{Nodes: []core.Node{
		{ID: "dns", Type: core.NodeTypeDNS, Resolution: core.ResolutionKnown, Provenance: prov},
		{ID: "db", Type: core.NodeTypeManagedDatabase, Resolution: core.ResolutionKnown, Provenance: prov},
	}}
}

func TestEvaluateJourneyLoadReadiness_MissingPeakRPS_NotAssessable(t *testing.T) {
	prov := core.NewProvenance(core.KindDerived, "test")
	j := core.DeclaredJourney{ID: "j1", Path: []string{"dns", "db"}, SteadyRPS: floatPtr(10)}
	env := core.EvaluateJourneyLoadReadiness(testIR(), core.Workload{}, j, prov)
	if env.State != core.AssessmentStateNotAssessable {
		t.Fatalf("State = %v, want not_assessable", env.State)
	}
	if !strings.Contains(env.Reason, "peak_rps") {
		t.Errorf("Reason = %q, want it to name peak_rps", env.Reason)
	}
}

func TestEvaluateJourneyLoadReadiness_MissingSteadyRPS_NotAssessable(t *testing.T) {
	prov := core.NewProvenance(core.KindDerived, "test")
	j := core.DeclaredJourney{ID: "j1", Path: []string{"dns", "db"}, PeakRPS: floatPtr(10)}
	env := core.EvaluateJourneyLoadReadiness(testIR(), core.Workload{}, j, prov)
	if env.State != core.AssessmentStateNotAssessable {
		t.Fatalf("State = %v, want not_assessable", env.State)
	}
	if !strings.Contains(env.Reason, "steady_rps") {
		t.Errorf("Reason = %q, want it to name steady_rps", env.Reason)
	}
}

func TestEvaluateJourneyLoadReadiness_UnknownComponent_NotAssessable(t *testing.T) {
	prov := core.NewProvenance(core.KindDerived, "test")
	j := core.DeclaredJourney{ID: "j1", Path: []string{"dns", "nonexistent"}, PeakRPS: floatPtr(10), SteadyRPS: floatPtr(5)}
	workload := core.Workload{Capacity: map[string]float64{core.JourneyCapacityKey(core.NodeTypeDNS): 100}}
	env := core.EvaluateJourneyLoadReadiness(testIR(), workload, j, prov)
	if env.State != core.AssessmentStateNotAssessable {
		t.Fatalf("State = %v, want not_assessable", env.State)
	}
	if !strings.Contains(env.Reason, "nonexistent") {
		t.Errorf("Reason = %q, want it to name the unknown component", env.Reason)
	}
}

func TestEvaluateJourneyLoadReadiness_MissingCapacity_NotAssessable(t *testing.T) {
	prov := core.NewProvenance(core.KindDerived, "test")
	j := core.DeclaredJourney{ID: "j1", Path: []string{"dns", "db"}, PeakRPS: floatPtr(10), SteadyRPS: floatPtr(5)}
	workload := core.Workload{Capacity: map[string]float64{core.JourneyCapacityKey(core.NodeTypeDNS): 100}}
	// db (managed_database) has no capacity entry.
	env := core.EvaluateJourneyLoadReadiness(testIR(), workload, j, prov)
	if env.State != core.AssessmentStateNotAssessable {
		t.Fatalf("State = %v, want not_assessable", env.State)
	}
	if !strings.Contains(env.Reason, "db") || !strings.Contains(env.Reason, core.JourneyCapacityKey(core.NodeTypeManagedDatabase)) {
		t.Errorf("Reason = %q, want it to name the component and the expected capacity key", env.Reason)
	}
}

func TestEvaluateJourneyLoadReadiness_FullyDeclared_Assessed(t *testing.T) {
	prov := core.NewProvenance(core.KindDerived, "test")
	j := core.DeclaredJourney{ID: "j1", Path: []string{"dns", "db"}, PeakRPS: floatPtr(10), SteadyRPS: floatPtr(5)}
	workload := core.Workload{Capacity: map[string]float64{
		core.JourneyCapacityKey(core.NodeTypeDNS):             100,
		core.JourneyCapacityKey(core.NodeTypeManagedDatabase): 50,
	}}
	env := core.EvaluateJourneyLoadReadiness(testIR(), workload, j, prov)
	if env.State != core.AssessmentStateAssessed {
		t.Fatalf("State = %v, want assessed: %+v", env.State, env)
	}
}

// TestJourneyCapacityKey_MatchesRealNodeTypeStrings proves the convention actually
// resolves against core.NodeType's own real values (not an invented parallel
// vocabulary) — e.g. "queue/stream" (a NodeType containing a slash) must still
// produce a valid, distinguishable key.
func TestJourneyCapacityKey_MatchesRealNodeTypeStrings(t *testing.T) {
	cases := map[core.NodeType]string{
		core.NodeTypeManagedDatabase: "managed_database_rps",
		core.NodeTypeQueueStream:     "queue/stream_rps",
	}
	for nodeType, want := range cases {
		if got := core.JourneyCapacityKey(nodeType); got != want {
			t.Errorf("JourneyCapacityKey(%v) = %q, want %q", nodeType, got, want)
		}
	}
}
