package core_test

// This file proves the full pipeline end to end using the ACTUAL frozen fixture files
// (golden/fixtures/*.findings.json), not a hand-wired path: load findings -> build a
// Scorecard -> diff two Scorecards -> get a real Assurance Delta.

import (
	"encoding/json"
	"os"
	"testing"

	"preflight/core"
)

func loadFixtureFindings(t *testing.T, path string) []core.Finding {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var findings []core.Finding
	if err := json.Unmarshal(data, &findings); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	return findings
}

func TestScorecardAndDelta_AgainstFrozenFixtureFiles(t *testing.T) {
	brokenFindings := loadFixtureFindings(t, "../golden/fixtures/aws-broken.findings.json")
	cleanFindings := loadFixtureFindings(t, "../golden/fixtures/aws.findings.json")

	brokenScorecard := core.BuildScorecard(brokenFindings, 1)
	cleanScorecard := core.BuildScorecard(cleanFindings, 2)

	if err := brokenScorecard.Validate(); err != nil {
		t.Fatalf("broken scorecard failed validation: %v", err)
	}
	if err := cleanScorecard.Validate(); err != nil {
		t.Fatalf("clean scorecard failed validation: %v", err)
	}

	// Every finding becomes a scorecard entry now (PC-83) — not_assessable findings get
	// the literal status string "not_assessable" rather than being excluded, so
	// ComputeDelta can tell "existed all along, only now assessable" apart from "brand
	// new". Some entries (the zone-kill findings' free-form "2 component(s) affected"
	// shape) still aren't independently rankable and correctly fall to not_assessable,
	// which the assertions below confirm rather than assume.
	prov := core.NewProvenance(core.KindDerived, "test")
	entries := core.ComputeDelta(brokenScorecard.StatusMap(), cleanScorecard.StatusMap(), prov)

	foundRDS, foundRPO := false, false
	for _, e := range entries {
		switch e.FindingID {
		case "finding.compliance.rds-storage-encryption.aws_db_instance.payments":
			foundRDS = true
			if e.Kind != core.DeltaImprovement {
				t.Errorf("RDS storage-encryption finding: Kind = %q, want improvement (a known-unsatisfied state got better)", e.Kind)
			}
		case "finding.compliance.rds-rpo-feasibility.aws_db_instance.payments":
			foundRPO = true
			// PC-83's own regression case: this finding is not_assessable in the broken
			// bundle (multi_az=false gives replication mode "none", no known RPO rule)
			// and satisfied in the clean one (multi_az=true) — an unknown resolving into
			// a known-good state, not a known state improving. Before the fix this came
			// back as "improvement"; see docs/PC-28-ITERATION-BOUND.md's own transcript
			// and demo/pc28-live-agent-run/TRANSCRIPT.md for where this was caught live.
			if e.Kind != core.DeltaResolvedRisk {
				t.Errorf("RDS RPO-feasibility finding: Kind = %q, want resolved_risk — not improvement, this was an unknown resolving into known-good, not a known state getting better", e.Kind)
			}
			if e.OldStatus != "not_assessable" || e.NewStatus != "satisfied" {
				t.Errorf("RDS RPO-feasibility finding: OldStatus/NewStatus = %q/%q, want not_assessable/satisfied", e.OldStatus, e.NewStatus)
			}
		}
		if e.Provenance.IsZero() {
			t.Errorf("entry %s has zero provenance", e.FindingID)
		}
	}
	if !foundRDS {
		t.Fatal("expected the RDS storage-encryption finding in the delta")
	}
	if !foundRPO {
		t.Fatal("expected the RDS RPO-feasibility finding in the delta")
	}
}
