package core_test

// This file hand-verifies core.BuildFindings' two newest findings (PC-28) against
// both real golden bundles — the NAT-gateway-redundancy check and the RDS RPO-
// feasibility check — the two additions found necessary while discovering that only
// one of the original three findings was actually delta-visible between bundles.

import (
	"testing"

	"preflight/core"
	"preflight/ingest"
	awsprovider "preflight/providers/aws"
)

func buildAllFindings(t *testing.T, dir string) []core.Finding {
	t.Helper()
	reg, err := awsprovider.Load()
	if err != nil {
		t.Fatalf("providers/aws.Load(): %v", err)
	}
	workload, err := ingest.LoadWorkload("../golden/workload.yaml")
	if err != nil {
		t.Fatalf("LoadWorkload: %v", err)
	}
	result, err := ingest.Ingest(dir, reg, 1)
	if err != nil {
		t.Fatalf("Ingest(%s): %v", dir, err)
	}
	return core.BuildFindings(result.IR, workload)
}

func findingByIDIn(t *testing.T, findings []core.Finding, id string) core.Finding {
	t.Helper()
	for _, f := range findings {
		if f.ID == id {
			return f
		}
	}
	t.Fatalf("finding %q not found among %d findings", id, len(findings))
	return core.Finding{}
}

func TestNATRedundancyFinding_AgainstGoldenBundles(t *testing.T) {
	broken := findingByIDIn(t, buildAllFindings(t, "../golden/aws-broken"), "finding.compliance.nat-gateway-redundancy")
	if broken.Outcome.Value != "unsatisfied" {
		t.Errorf("broken: Outcome.Value = %v, want unsatisfied (defect 1: only nat_a remains)", broken.Outcome.Value)
	}
	if err := broken.Validate(); err != nil {
		t.Errorf("broken finding failed schema validation: %v", err)
	}

	clean := findingByIDIn(t, buildAllFindings(t, "../golden/aws"), "finding.compliance.nat-gateway-redundancy")
	if clean.Outcome.Value != "satisfied" {
		t.Errorf("clean: Outcome.Value = %v, want satisfied (one NAT gateway per AZ)", clean.Outcome.Value)
	}
	if err := clean.Validate(); err != nil {
		t.Errorf("clean finding failed schema validation: %v", err)
	}
}

func TestRDSRPOFeasibilityFinding_AgainstGoldenBundles(t *testing.T) {
	broken := findingByIDIn(t, buildAllFindings(t, "../golden/aws-broken"), "finding.compliance.rds-rpo-feasibility.aws_db_instance.payments")
	if broken.Outcome.State != core.AssessmentStateNotAssessable {
		t.Errorf("broken: Outcome.State = %q, want not_assessable — multi_az=false means replication mode \"none\", which has no RPO feasibility rule (PC-14's own deliberate design, not re-litigated here)", broken.Outcome.State)
	}

	clean := findingByIDIn(t, buildAllFindings(t, "../golden/aws"), "finding.compliance.rds-rpo-feasibility.aws_db_instance.payments")
	if clean.Outcome.Value != "satisfied" {
		t.Errorf("clean: Outcome.Value = %v, want satisfied — multi_az=true gives sync replication, which satisfies any declared RPO including 0", clean.Outcome.Value)
	}
	if err := clean.Validate(); err != nil {
		t.Errorf("clean finding failed schema validation: %v", err)
	}
}

// TestBuildFindings_NowHasThreeDeltaVisibleFindings is the structural confirmation
// that PC-28's output-shape fix actually worked: 3 of 5 findings now differ
// meaningfully between the broken and clean bundles (up from 1 of 3 originally).
func TestBuildFindings_NowHasThreeDeltaVisibleFindings(t *testing.T) {
	broken := buildAllFindings(t, "../golden/aws-broken")
	clean := buildAllFindings(t, "../golden/aws")

	brokenByID := map[string]any{}
	for _, f := range broken {
		brokenByID[f.ID] = f.Outcome.Value
	}
	differing := 0
	for _, f := range clean {
		if v, ok := brokenByID[f.ID]; ok && v != f.Outcome.Value {
			differing++
		}
	}
	if differing < 3 {
		t.Errorf("only %d findings differ between bundles, want at least 3 (nat-gateway-redundancy, rds-storage-encryption, and the RPO finding's state even though its VALUE comparison won't show as differing due to not_assessable on one side — recount if this changes)", differing)
	}
}
