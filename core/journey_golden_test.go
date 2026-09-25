package core_test

// PC-124: proves the real golden/workload.yaml journeys behave exactly as documented
// — "checkout" fully assessed, "settlement" a real, checked not_assessable case.

import (
	"path/filepath"
	"testing"

	"preflight/core"
	"preflight/ingest"
)

func loadGoldenWorkload(t *testing.T) core.Workload {
	t.Helper()
	path, err := filepath.Abs("../golden/workload.yaml")
	if err != nil {
		t.Fatal(err)
	}
	w, err := ingest.LoadWorkload(path)
	if err != nil {
		t.Fatalf("LoadWorkload: %v", err)
	}
	return w
}

func TestGoldenWorkload_CheckoutJourney_FullyAssessed(t *testing.T) {
	ir := realGoldenIR(t)
	workload := loadGoldenWorkload(t)
	prov := core.NewProvenance(core.KindDerived, "test")

	var checkout *core.DeclaredJourney
	for i := range workload.Journeys {
		if workload.Journeys[i].ID == "checkout" {
			checkout = &workload.Journeys[i]
		}
	}
	if checkout == nil {
		t.Fatal("expected a \"checkout\" journey in golden/workload.yaml")
	}

	env := core.EvaluateJourneyLoadReadiness(ir, workload, *checkout, prov)
	if env.State != core.AssessmentStateAssessed {
		t.Fatalf("checkout journey: got %+v, want assessed (every path component has a matching capacity entry)", env)
	}
}

func TestGoldenWorkload_SettlementJourney_NotAssessable(t *testing.T) {
	ir := realGoldenIR(t)
	workload := loadGoldenWorkload(t)
	prov := core.NewProvenance(core.KindDerived, "test")

	var settlement *core.DeclaredJourney
	for i := range workload.Journeys {
		if workload.Journeys[i].ID == "settlement" {
			settlement = &workload.Journeys[i]
		}
	}
	if settlement == nil {
		t.Fatal("expected a \"settlement\" journey in golden/workload.yaml")
	}
	if settlement.PeakRPS != nil || settlement.SteadyRPS != nil {
		t.Fatal("settlement journey must NOT declare peak_rps/steady_rps — that's the real, checked not_assessable case this test proves")
	}

	env := core.EvaluateJourneyLoadReadiness(ir, workload, *settlement, prov)
	if env.State != core.AssessmentStateNotAssessable {
		t.Fatalf("settlement journey: got %+v, want not_assessable (no declared rps)", env)
	}
}
