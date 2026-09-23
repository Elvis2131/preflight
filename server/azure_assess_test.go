package server_test

// PC-29: proves server.Assess itself — not just cmd/gen-golden-fixtures — actually
// works against an Azure bundle. Found the hard way: server.Assess hardcoded AWS's
// own provider registry (awsprovider.Load()), so calling it against golden/azure
// failed the Minimum Viable Graph check outright — every Azure resource type was
// unrecognized, producing zero real nodes. Fixed by merging both providers'
// registries (providers.Merge) rather than adding a provider-selection field to
// AssessRequest — resource_type strings are already provider-namespaced
// (aws_*/azurerm_*), so a merge is unambiguous and the caller never needs to declare
// which cloud a bundle is.

import (
	"path/filepath"
	"testing"

	"preflight/server"
)

func TestAssess_AgainstGoldenAzureBundle_WorksThroughTheMergedRegistry(t *testing.T) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	bundleDir, err := filepath.Abs("../golden/azure")
	if err != nil {
		t.Fatal(err)
	}
	workloadPath, err := filepath.Abs("../golden/workload.yaml")
	if err != nil {
		t.Fatal(err)
	}

	resp, err := server.Assess(store, server.AssessRequest{
		SessionID:    "azure-server-test",
		BundleDir:    bundleDir,
		WorkloadPath: workloadPath,
	})
	if err != nil {
		t.Fatalf("Assess against golden/azure: %v", err)
	}

	if len(resp.Findings) == 0 {
		t.Fatal("expected real findings against golden/azure, got none")
	}

	found := false
	for _, f := range resp.Findings {
		if f.ID == "finding.compliance.sql-storage-encryption.azurerm_mssql_database.payments" {
			found = true
			if f.Outcome.Value != "satisfied" {
				t.Errorf("Azure SQL storage-encryption: Outcome.Value = %v, want satisfied", f.Outcome.Value)
			}
		}
	}
	if !found {
		t.Fatal("expected the Azure SQL storage-encryption finding in the response")
	}
}
