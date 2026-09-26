package core_test

// PC-130's own acceptance criterion: "Fault results appear as findings with correct
// four-dimension values and flow into delta/report."

import (
	"testing"

	"preflight/core"
)

func TestBuildConfigurationBlastSurfaceFindings_UnsatisfiedWhenFragile(t *testing.T) {
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
	if f.Outcome.Value != string(core.ComplianceUnsatisfied) {
		t.Fatalf("Outcome.Value = %q, want %q", f.Outcome.Value, core.ComplianceUnsatisfied)
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
