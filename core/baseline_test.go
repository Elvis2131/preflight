package core_test

// PC-161: a fault simulation is only meaningful for traffic that flowed before the fault. These tests use
// the real Rung 3 bundle (two web instances behind a load balancer) through the real ingest pipeline and
// simulate losing one instance, with the baseline healthy, deliberately broken, and not declared at all.

import (
	"path/filepath"
	"strings"
	"testing"

	"preflight/core"
	"preflight/ingest"

	awsprovider "preflight/providers/aws"
)

func rung3Design(t *testing.T) (*core.IR, core.Workload) {
	t.Helper()
	reg, err := awsprovider.Load()
	if err != nil {
		t.Fatal(err)
	}
	res, err := ingest.Ingest(filepath.Join("..", "validate", "rung3", "terraform"), reg, 1)
	if err != nil || res.Insufficient != nil {
		t.Fatalf("ingest: %v %+v", err, res.Insufficient)
	}
	w, err := ingest.LoadWorkload(filepath.Join("..", "validate", "rung3", "workload.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return res.IR, w
}

func loseWebA(ir *core.IR, w core.Workload) core.SimulateResponse {
	return core.Simulate(ir, w, []core.Fault{{Type: "node_loss", Target: "aws_instance.web_a"}}, core.NewProvenance(core.KindDerived, "test"))
}

func baselineOf(t *testing.T, r core.SimulateResponse, id string) core.JourneyBaseline {
	t.Helper()
	for _, j := range r.Baseline.Journeys {
		if j.JourneyID == id {
			return j
		}
	}
	t.Fatalf("no baseline entry for journey %q in %+v", id, r.Baseline.Journeys)
	return core.JourneyBaseline{}
}

func TestBaseline_WorkingDesign_SurvivorFlowsAndLostPathIsBrokenByTheFault(t *testing.T) {
	ir, w := rung3Design(t)
	r := loseWebA(ir, w)
	if got := baselineOf(t, r, "via-b").Status; got != core.BaselineFlowsBeforeAndAfter {
		t.Errorf("via-b = %s, want flows_before_and_after", got)
	}
	a := baselineOf(t, r, "via-a")
	if a.Status != core.BaselineBrokenByFault || a.AfterFaultBlockedAt != "aws_instance.web_a" {
		t.Errorf("via-a = %+v, want broken_by_fault at aws_instance.web_a", a)
	}
	if v := r.Baseline.Summary.Value; r.Baseline.Summary.State != core.AssessmentStateAssessed || v != core.BaselineAllDeclaredJourneysFlow {
		t.Errorf("summary = %+v, want assessed all_declared_journeys_flow", r.Baseline.Summary)
	}
	if r.Verdict.State != core.AssessmentStateAssessed || r.Verdict.Value != "unaffected" {
		t.Errorf("a working design's structural verdict is unchanged: %+v", r.Verdict)
	}
}

// The case that came back "unaffected" before PC-161: the survivor's journey was declared on a port its
// security group does not admit, so it never carried traffic.
func TestBaseline_BrokenBaseline_IsNeverCountedAsSurvived(t *testing.T) {
	ir, w := rung3Design(t)
	for i := range w.Journeys {
		if w.Journeys[i].ID == "via-b" {
			w.Journeys[i].Port = 9999
		}
	}
	r := loseWebA(ir, w)
	b := baselineOf(t, r, "via-b")
	if b.Status != core.BaselineAlreadyBlocked || b.BaselineBlockedAt != "aws_lb.main" || !strings.Contains(b.BaselineReason, "sg_dest_ingress") {
		t.Fatalf("via-b = %+v, want already_blocked at aws_lb.main (sg_dest_ingress)", b)
	}
	if r.Baseline.Summary.Value != core.BaselineSomeDeclaredJourneysBlocked {
		t.Errorf("summary = %+v", r.Baseline.Summary)
	}
	if r.Verdict.State != core.AssessmentStateNotAssessable || !strings.Contains(r.Verdict.Reason, `journey "via-b" was already blocked`) {
		t.Errorf("the headline must not say unaffected for a design that never carried the journey: %+v", r.Verdict)
	}
	// Structural facts stay available on a design that does not work yet.
	if len(r.Journeys) == 0 || len(r.Cascade) == 0 {
		t.Errorf("structural results must still be reported: journeys=%v cascade=%v", r.Journeys, r.Cascade)
	}
}

func TestBaseline_NoJourneysDeclared_SaysNothingWasValidated(t *testing.T) {
	ir, w := rung3Design(t)
	w.Journeys = nil
	r := loseWebA(ir, w)
	if r.Baseline.Summary.State != core.AssessmentStateNotAssessable || !strings.Contains(r.Baseline.Summary.Reason, "no journey is declared") {
		t.Errorf("summary = %+v, want not_assessable naming that no journey is declared", r.Baseline.Summary)
	}
	if r.Baseline.Journeys == nil || len(r.Baseline.Journeys) != 0 {
		t.Errorf("journeys must be an empty array, never null: %#v", r.Baseline.Journeys)
	}
	// The verdict stays the structural reachability answer; the summary above is what says it checked no traffic.
	if r.Verdict.Value != "unaffected" {
		t.Errorf("structural verdict = %+v", r.Verdict)
	}
}

// A hop the engine cannot decide is "could not be checked", never "blocked": golden's settlement journey
// stops at a queue with no resolvable route table.
func TestBaseline_UndecidableHop_IsNotAssessableNotBlocked(t *testing.T) {
	ir := realGoldenIR(t)
	w := loadGoldenWorkload(t)
	r := core.Simulate(ir, w, []core.Fault{{Type: "node_loss", Target: "aws_eks_cluster.payments"}}, core.NewProvenance(core.KindDerived, "test"))
	s := baselineOf(t, r, "settlement")
	if s.Status != core.BaselineNotAssessable || !strings.Contains(s.BaselineReason, "no resolvable effective route table") {
		t.Errorf("settlement = %+v, want not_assessable (no resolvable route table), not already_blocked", s)
	}
}

// Each of the five statuses, from hand-built before/after results.
func TestComputeBaselineValidity_AllFiveStatuses(t *testing.T) {
	prov := core.NewProvenance(core.KindDerived, "test")
	w := core.Workload{Journeys: []core.DeclaredJourney{{ID: "ok"}, {ID: "blocked"}, {ID: "broken"}, {ID: "degraded"}, {ID: "undecided"}}}
	before := []core.JourneyFlowResult{
		{JourneyID: "ok", Flows: true}, {JourneyID: "blocked", BlockedAt: "x", BlockedReason: "denied"},
		{JourneyID: "broken", Flows: true}, {JourneyID: "degraded", Flows: true},
		{JourneyID: "undecided", BlockedAt: "y", BlockedReason: "unreadable", NotAssessable: true},
	}
	after := []core.JourneyFlowResult{
		{JourneyID: "ok", Flows: true}, {JourneyID: "blocked", BlockedAt: "x", BlockedReason: "denied"},
		{JourneyID: "broken", BlockedAt: "z", BlockedReason: "gone"},
		{JourneyID: "degraded", BlockedAt: "z", BlockedReason: "gone", Degraded: true, DegradedVia: "fallback"},
		{JourneyID: "undecided", BlockedAt: "y", BlockedReason: "unreadable", NotAssessable: true},
	}
	got := core.ComputeBaselineValidity(w, before, after, prov)
	want := map[string]core.JourneyBaselineStatus{
		"ok": core.BaselineFlowsBeforeAndAfter, "blocked": core.BaselineAlreadyBlocked, "broken": core.BaselineBrokenByFault,
		"degraded": core.BaselineDegradedByFault, "undecided": core.BaselineNotAssessable,
	}
	for _, j := range got.Journeys {
		if j.Status != want[j.JourneyID] {
			t.Errorf("%s = %s, want %s", j.JourneyID, j.Status, want[j.JourneyID])
		}
	}
	// NEGATIVE CONTROL: ignoring the baseline (using the after-fault result as "before") turns the
	// already-blocked journey into something else, so the cases above really do depend on it.
	ignored := core.ComputeBaselineValidity(w, after, after, prov)
	for _, j := range ignored.Journeys {
		if j.JourneyID == "broken" && j.Status == core.BaselineBrokenByFault {
			t.Error("with the baseline ignored a journey broken by the fault must not still read as broken_by_fault")
		}
	}
}
