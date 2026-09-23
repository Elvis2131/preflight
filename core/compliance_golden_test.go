package core_test

// This file is PC-18's first acceptance criterion: "at least one control is
// implemented and passes/fails correctly against the golden fixture" — checked
// against BOTH bundles, not just one, so both the pass and the fail path are proven
// against real data rather than a synthetic true/false pair.

import (
	"testing"

	"preflight/core"
	"preflight/ingest"
	awsprovider "preflight/providers/aws"
)

func buildRDSEncryptionFinding(t *testing.T, dir string) core.Finding {
	t.Helper()
	reg, err := awsprovider.Load()
	if err != nil {
		t.Fatalf("providers/aws.Load(): %v", err)
	}
	result, err := ingest.Ingest(dir, reg, 1)
	if err != nil {
		t.Fatalf("Ingest(%s): %v", dir, err)
	}
	rds := findNode(t, result.IR.Nodes, "aws_db_instance.payments")

	var storageEncrypted *bool
	if v, ok := rds.RawAttributes["storage_encrypted"].(bool); ok {
		storageEncrypted = &v
	}

	prov := core.NewProvenance(core.KindDerived, "core/analyse:rds-storage-encryption-check")
	result2, rationale := core.StorageEncryptionCheck(storageEncrypted, prov)

	attr := "storage_encrypted"
	nodeID := rds.ID

	outcomeState := core.AssessmentStateAssessed
	var outcomeValue any = string(result2.Status)
	if result2.Status == core.ComplianceNotAssessable {
		outcomeState = core.AssessmentStateNotAssessable
	}

	f := core.Finding{
		ID:    "finding.rds-storage-encryption." + rds.ID,
		Title: "RDS storage encryption at rest",
		Dimensions: core.FailureMode{
			Trigger:            "unencrypted storage exposed via snapshot, backup, or underlying storage compromise",
			AffectedComponents: []string{nodeID},
			Detection:          core.DetectionModeled,
			Impact:             core.NotAssessable[any]("impact dimension not evaluated by this compliance check — see PC-17's FailureMode for structural/failure-mode findings; this is a compliance finding", prov).ToEnvelope(),
			Likelihood:         core.DeriveLikelihood(prov).ToEnvelope(),
			Detectability:      core.Assessed[any]("high — this simulator models the attribute directly", prov).ToEnvelope(),
			Recoverability: core.Recoverability{
				FailoverPathExists: core.NotAssessable[any]("recoverability not evaluated by this compliance check", prov).ToEnvelope(),
				RPOFeasible:        core.NotAssessable[any]("recoverability not evaluated by this compliance check", prov).ToEnvelope(),
			},
		},
		Evidence: []core.EvidenceRef{
			{
				NodeID:      &nodeID,   // resource address
				Attribute:   &attr,     // attribute
				Description: rationale, // rationale
			},
		},
		Outcome: core.AssessmentEnvelope{
			State:      outcomeState,
			Value:      outcomeValue,
			Provenance: prov,
		},
	}
	if outcomeState == core.AssessmentStateNotAssessable {
		f.Outcome.Value = nil
		f.Outcome.Reason = rationale
	}
	return f
}

func TestComplianceCheck_CleanBundle_RDSEncrypted_Satisfied(t *testing.T) {
	f := buildRDSEncryptionFinding(t, "../golden/aws")
	if err := f.Validate(); err != nil {
		t.Fatalf("Finding failed its own frozen schema validation: %v", err)
	}

	// Criterion 3: never a bare pass/fail.
	if f.Outcome.Value != string(core.ComplianceSatisfied) {
		t.Fatalf("Outcome.Value = %v, want %q — a 5-value ComplianceStatus, never a bare bool", f.Outcome.Value, core.ComplianceSatisfied)
	}

	// Criterion 2: resource address, attribute, rationale.
	ev := f.Evidence[0]
	if ev.NodeID == nil || *ev.NodeID != "aws_db_instance.payments" {
		t.Errorf("Evidence.NodeID (resource address) = %v, want aws_db_instance.payments", ev.NodeID)
	}
	if ev.Attribute == nil || *ev.Attribute != "storage_encrypted" {
		t.Errorf("Evidence.Attribute = %v, want storage_encrypted", ev.Attribute)
	}
	if ev.Description == "" {
		t.Error("Evidence.Description (rationale) must be non-empty")
	}
}

func TestComplianceCheck_BrokenBundle_RDSUnencrypted_Unsatisfied(t *testing.T) {
	f := buildRDSEncryptionFinding(t, "../golden/aws-broken")
	if err := f.Validate(); err != nil {
		t.Fatalf("Finding failed its own frozen schema validation: %v", err)
	}

	if f.Outcome.Value != string(core.ComplianceUnsatisfied) {
		t.Fatalf("Outcome.Value = %v, want %q — golden/aws-broken's defect 2 sets storage_encrypted = false", f.Outcome.Value, core.ComplianceUnsatisfied)
	}

	ev := f.Evidence[0]
	if ev.NodeID == nil || *ev.NodeID != "aws_db_instance.payments" {
		t.Errorf("Evidence.NodeID = %v, want aws_db_instance.payments", ev.NodeID)
	}
	if ev.Attribute == nil || *ev.Attribute != "storage_encrypted" {
		t.Errorf("Evidence.Attribute = %v, want storage_encrypted", ev.Attribute)
	}
	if ev.Description == "" {
		t.Error("Evidence.Description must be non-empty")
	}
}
