package core_test

// PC-29: hand-verifies that BuildFindings' storage-encryption and RPO-feasibility
// checks now run against Azure's own managed_database node, not just AWS's hardcoded
// one — the fix that unblocks an Azure iteration cycle at all. Also confirms AWS's own
// finding IDs/values stayed byte-identical (findings_builder_golden_test.go's own
// existing tests already assert this — this file only adds Azure's own new coverage).

import (
	"testing"

	"preflight/core"
	"preflight/ingest"
	azureprovider "preflight/providers/azure"
)

func buildAllAzureFindings(t *testing.T, dir string) []core.Finding {
	t.Helper()
	reg, err := azureprovider.Load()
	if err != nil {
		t.Fatalf("providers/azure.Load(): %v", err)
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

func TestStorageEncryptionAndRPOFindings_AgainstGoldenAzureBundle(t *testing.T) {
	findings := buildAllAzureFindings(t, "../golden/azure")

	enc := findingByIDIn(t, findings, "finding.compliance.sql-storage-encryption.azurerm_mssql_database.payments")
	if enc.Outcome.Value != "satisfied" {
		t.Errorf("storage-encryption: Outcome.Value = %v, want satisfied (transparent_data_encryption_enabled=true)", enc.Outcome.Value)
	}
	if enc.Title != "Azure SQL storage encryption at rest" {
		t.Errorf("storage-encryption: Title = %q, want the Azure-labeled title, not AWS's \"RDS...\"", enc.Title)
	}
	if err := enc.Validate(); err != nil {
		t.Errorf("storage-encryption finding failed schema validation: %v", err)
	}

	rpo := findingByIDIn(t, findings, "finding.compliance.sql-rpo-feasibility.azurerm_mssql_database.payments")
	if rpo.Outcome.Value != "satisfied" {
		t.Errorf("rpo-feasibility: Outcome.Value = %v, want satisfied (zone_redundant=true gives sync replication)", rpo.Outcome.Value)
	}
	if err := rpo.Validate(); err != nil {
		t.Errorf("rpo-feasibility finding failed schema validation: %v", err)
	}
}

// TestAssuranceDelta_AgainstGoldenAzureBundles is PC-29's actual proof: the delta
// engine (PC-83's fix included) produces a real, correctly-classified iteration cycle
// on Azure, not just AWS — a second cloud exercising the exact same resolved_risk/
// improvement distinction PC-83 fixed.
func TestAssuranceDelta_AgainstGoldenAzureBundles(t *testing.T) {
	broken := buildAllAzureFindings(t, "../golden/azure-broken")
	clean := buildAllAzureFindings(t, "../golden/azure")

	brokenSC := core.BuildScorecard(broken, 1)
	cleanSC := core.BuildScorecard(clean, 2)
	prov := core.NewProvenance(core.KindDerived, "test")
	entries := core.ComputeDelta(brokenSC.StatusMap(), cleanSC.StatusMap(), prov)

	foundEnc, foundRPO := false, false
	for _, e := range entries {
		switch e.FindingID {
		case "finding.compliance.sql-storage-encryption.azurerm_mssql_database.payments":
			foundEnc = true
			if e.Kind != core.DeltaImprovement {
				t.Errorf("storage-encryption: Kind = %q, want improvement (unsatisfied -> satisfied)", e.Kind)
			}
		case "finding.compliance.sql-rpo-feasibility.azurerm_mssql_database.payments":
			foundRPO = true
			if e.Kind != core.DeltaResolvedRisk {
				t.Errorf("rpo-feasibility: Kind = %q, want resolved_risk (not_assessable -> satisfied)", e.Kind)
			}
		}
		if e.Provenance.IsZero() {
			t.Errorf("entry %s has zero provenance", e.FindingID)
		}
	}
	if !foundEnc {
		t.Fatal("expected the Azure SQL storage-encryption finding in the delta")
	}
	if !foundRPO {
		t.Fatal("expected the Azure SQL RPO-feasibility finding in the delta")
	}
}

// TestAWSFindingIDs_UnchangedAfterGenericization is the negative-space check: proves
// the genericization didn't accidentally rename or reshape AWS's own already-tested,
// already-documented finding IDs by running the exact same golden/aws bundle through
// the now-generic BuildFindings and confirming the IDs are byte-identical to what
// core/scorecard_golden_test.go and demo/pc28-live-agent-run/TRANSCRIPT.md already
// depend on.
func TestAWSFindingIDs_UnchangedAfterGenericization(t *testing.T) {
	findings := buildAllFindings(t, "../golden/aws")
	wantIDs := []string{
		"finding.compliance.rds-storage-encryption.aws_db_instance.payments",
		"finding.compliance.rds-rpo-feasibility.aws_db_instance.payments",
	}
	for _, id := range wantIDs {
		f := findingByIDIn(t, findings, id)
		if f.ID != id {
			t.Errorf("got ID %q, want exactly %q — genericization must not rename AWS's own established finding IDs", f.ID, id)
		}
	}
}
