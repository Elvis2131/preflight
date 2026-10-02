package core_test

// PC-130's own acceptance criterion: "Fault results appear as findings with correct
// four-dimension values and flow into delta/report."

import (
	"strings"
	"testing"

	"preflight/core"
)

// The blast surface is reported as a count, never as a verdict: a working least-privilege
// journey always has load-bearing rules, and that must not read as 'unsatisfied'.
func TestBuildConfigurationBlastSurfaceFindings_DescriptiveNotAVerdict(t *testing.T) {
	ir := buildFlowTestIR(true, true)
	workload := core.Workload{
		Journeys: []core.DeclaredJourney{
			{ID: "j1", Name: "j1", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432, Criticality: "tier1"},
		},
	}

	findings := core.BuildConfigurationBlastSurfaceFindings(ir, workload)
	f := findFinding(findings, "finding.resilience.configuration-blast-surface.j1")
	if f == nil {
		t.Fatal("expected a configuration-blast-surface finding for j1")
	}
	value, _ := f.Outcome.Value.(string)
	if f.Outcome.State != core.AssessmentStateAssessed || !strings.Contains(value, "would break it if changed alone") {
		t.Fatalf("Outcome = %+v, want an assessed, descriptive count of the rules that would break the journey", f.Outcome)
	}
	for _, verdict := range []string{"unsatisfied", "satisfied", "partial"} {
		if value == verdict {
			t.Fatalf("Outcome.Value = %q: a blast surface must never be a pass/fail verdict (a working design has load-bearing rules)", verdict)
		}
	}
	if f.Dimensions.Detection != core.DetectionUnknown {
		t.Fatalf("Detection = %q, want %q — the Card's own explicit instruction: a config change isn't detected by redundancy", f.Dimensions.Detection, core.DetectionUnknown)
	}
	if f.Dimensions.Detectability.State != core.AssessmentStateNotAssessable {
		t.Fatalf("Detectability.State = %q, want not_assessable — must not be defaulted to anything better", f.Dimensions.Detectability.State)
	}
	if len(f.Evidence) < 2 {
		t.Fatalf("got %d evidence entries, want a summary plus at least one breaking rule", len(f.Evidence))
	}
}

func TestBuildConfigurationBlastSurfaceFindings_NotAssessableWhenBaselineBroken(t *testing.T) {
	ir := buildFlowTestIR(false, true) // SG already denies at baseline
	workload := core.Workload{
		Journeys: []core.DeclaredJourney{
			{ID: "j1", Name: "j1", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432, Criticality: "tier1"},
		},
	}

	findings := core.BuildConfigurationBlastSurfaceFindings(ir, workload)
	f := findFinding(findings, "finding.resilience.configuration-blast-surface.j1")
	if f == nil {
		t.Fatal("expected a configuration-blast-surface finding for j1")
	}
	if f.Outcome.State != core.AssessmentStateNotAssessable {
		t.Fatalf("Outcome.State = %q, want not_assessable — a blast surface is not meaningful without a working baseline", f.Outcome.State)
	}
}

func TestBuildConfigurationBlastSurfaceFindings_NoJourneys_Empty(t *testing.T) {
	ir := buildFlowTestIR(true, true)
	findings := core.BuildConfigurationBlastSurfaceFindings(ir, core.Workload{})
	if len(findings) != 0 {
		t.Fatalf("got %d findings, want 0 — no journeys declared", len(findings))
	}
}
