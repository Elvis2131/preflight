package core_test

// This file is PC-19's first acceptance criterion, run against the real golden
// fixtures where they can prove it, and honestly split from where they can't.
//
// golden/aws-broken (Version 1, "before an agent fixes it") vs golden/aws (Version 2,
// "after") produces a genuine IMPROVEMENT: PC-18's RDS storage-encryption compliance
// finding goes from unsatisfied to satisfied — real data, not a constructed example.
//
// The two real bundles do NOT naturally produce a REGRESSION or a NEW_RISK
// simultaneously alongside that improvement: golden/aws-broken's eight defects were
// all deliberately built to make it uniformly worse than golden/aws, never mixed
// (nothing in the broken bundle is BETTER than the clean one in some other tracked
// dimension). Forcing a regression/new-risk example out of these two specific bundles
// would mean inventing a finding that isn't real. Those two DeltaKind values are
// instead hand-verified in core/internal/analyse/delta_test.go's
// TestComputeDelta_AllFiveKinds, against a small, realistic, hand-worked scorecard
// pair — the same "prove it small and by hand first" discipline as every other engine
// in this codebase (mincut's synthetic graphs, containment's synthetic hierarchies).
// This file's job is narrower and different: prove the real golden fixture, run
// through the real engine, produces at least the one category it actually can.
import (
	"testing"

	"preflight/core"
	"preflight/ingest"
	awsprovider "preflight/providers/aws"
)

func rdsEncryptionStatus(t *testing.T, dir string) string {
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
	prov := core.NewProvenance(core.KindDerived, "test")
	res, _ := core.StorageEncryptionCheck(storageEncrypted, prov)
	return string(res.Status)
}

func TestAssuranceDelta_AgainstGoldenBundles_RealImprovement(t *testing.T) {
	oldStatus := rdsEncryptionStatus(t, "../golden/aws-broken") // Version 1: unsatisfied
	newStatus := rdsEncryptionStatus(t, "../golden/aws")        // Version 2: satisfied

	if oldStatus != "unsatisfied" || newStatus != "satisfied" {
		t.Fatalf("precondition failed: old=%q new=%q, want unsatisfied -> satisfied (defect 2 fixed)", oldStatus, newStatus)
	}

	prov := core.NewProvenance(core.KindDerived, "core/analyse:assurance-delta")
	entries := core.ComputeDelta(
		map[string]string{"finding.compliance.rds-storage-encryption": oldStatus},
		map[string]string{"finding.compliance.rds-storage-encryption": newStatus},
		prov,
	)

	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	entry := entries[0]
	if entry.Kind != core.DeltaImprovement {
		t.Fatalf("Kind = %q, want improvement", entry.Kind)
	}
	if entry.Provenance.IsZero() {
		t.Fatal("entry has no provenance")
	}
	if err := entry.Validate(); err != nil {
		t.Fatalf("entry failed schema validation: %v", err)
	}
}
