package core_test

// PC-121's own acceptance criteria, each cited below:
//   - "PCI DSS 4 and SOC 2 catalogs defined with every control classified assessable
//     / partial / not-assessable-from-architecture"
//   - "Assessable controls implemented with resource-level evidence; golden bundles
//     produce hand-verified expected results per framework"
//   - "Report shows counts for all three classes; test asserts not-assessable
//     controls are never counted as satisfied"
//   - "Control count recorded against the ~100 OPA revisit trigger"

import (
	"testing"

	"preflight/core"
)

func TestPCIDSS4Catalog_AllTwelveRequirementsClassified(t *testing.T) {
	ir := &core.IR{}
	results := core.BuildPCIDSS4Catalog(ir, core.Workload{})
	counts := core.SummarizeComplianceCatalogCounts(results)

	if got := counts.Assessable + counts.Partial + counts.NotAssessable; got != 12 {
		t.Fatalf("got %d distinct controls, want 12 (PCI DSS v4.0's own 12 top-level requirements)", got)
	}
	if counts.Assessable != 2 {
		t.Errorf("Assessable = %d, want 2 (Requirement 1 network security, Requirement 3 stored data protection)", counts.Assessable)
	}
	if counts.NotAssessable != 10 {
		t.Errorf("NotAssessable = %d, want 10", counts.NotAssessable)
	}
}

func TestSOC2Catalog_AllNineCategoriesClassified(t *testing.T) {
	ir := &core.IR{}
	results := core.BuildSOC2Catalog(ir, core.Workload{})
	counts := core.SummarizeComplianceCatalogCounts(results)

	if got := counts.Assessable + counts.Partial + counts.NotAssessable; got != 9 {
		t.Fatalf("got %d distinct controls, want 9 (SOC 2's own nine Common Criteria categories)", got)
	}
	if counts.Partial != 1 {
		t.Errorf("Partial = %d, want 1 (CC6, logical access controls)", counts.Partial)
	}
	if counts.NotAssessable != 8 {
		t.Errorf("NotAssessable = %d, want 8", counts.NotAssessable)
	}
}

func TestComplianceCatalogs_TotalControlCount_WellUnderOPARevisitTrigger(t *testing.T) {
	// Design §4.6's own revisit trigger: "if the catalog approaches ~100 controls,
	// reassess Go registry vs OPA/Rego." Recorded here so a future ticket adding
	// controls trips this test long before the catalog silently grows past the
	// trigger unnoticed.
	ir := &core.IR{}
	total := len(core.BuildPCIDSS4Catalog(ir, core.Workload{})) +
		len(core.BuildSOC2Catalog(ir, core.Workload{})) +
		len(core.BuildCISAWSCatalog(ir, core.Workload{}))
	if total > 40 {
		t.Errorf("total control result count = %d — approaching Design §4.6's ~100 OPA/Rego revisit trigger sooner than expected; re-read that section before adding more", total)
	}
	t.Logf("current combined control result count: %d (well under the ~100 OPA/Rego revisit trigger)", total)
}

// TestComplianceCatalog_NoVerbatimControlText is a structural proxy for the Card's
// own licensing requirement ("no verbatim copyrighted control text stored; IDs and
// paraphrased titles only") — every title in this catalog is capped at a length no
// real standard's own full requirement text would fit in, catching an accidental
// paste of full control language before it ships.
func TestComplianceCatalog_NoVerbatimControlText(t *testing.T) {
	ir := &core.IR{}
	all := append(core.BuildPCIDSS4Catalog(ir, core.Workload{}), core.BuildSOC2Catalog(ir, core.Workload{})...)
	all = append(all, core.BuildCISAWSCatalog(ir, core.Workload{})...)
	for _, r := range all {
		if len(r.Title) > 120 {
			t.Errorf("control %s: Title is %d characters — too long for a short paraphrase, check it isn't verbatim standard text: %q", r.ControlID, len(r.Title), r.Title)
		}
	}
}

func TestSummarizeComplianceResults_NotAssessableNeverCountedSatisfied(t *testing.T) {
	ir := &core.IR{}
	all := append(core.BuildPCIDSS4Catalog(ir, core.Workload{}), core.BuildSOC2Catalog(ir, core.Workload{})...)
	counts := core.SummarizeComplianceResults(all)

	// An empty IR: every assessable control has no resource to evaluate, so every
	// single result — assessable, partial, and not-assessable-classified alike —
	// must land in NotAssessable, never Satisfied.
	if counts.Satisfied != 0 {
		t.Fatalf("Satisfied = %d, want 0 — an empty IR has nothing for any assessable control to find satisfied", counts.Satisfied)
	}
	if counts.NotAssessable == 0 {
		t.Fatal("NotAssessable = 0, want > 0")
	}

	// The structural guarantee: EVERY not_assessable_from_architecture-classified
	// control's own result, across both catalogs, is ComplianceNotAssessable —
	// never satisfied, by construction (notAssessableFromArchitectureResult has no
	// evaluation logic at all to produce anything else).
	for _, r := range all {
		if r.Classification == core.ClassificationNotAssessable && r.Result.Status == core.ComplianceSatisfied {
			t.Errorf("control %s is classified not_assessable_from_architecture but its own result is satisfied — structurally impossible unless this invariant was broken", r.ControlID)
		}
	}
}

func TestPCIDSS4_GoldenBundle_HandVerified(t *testing.T) {
	ir := realGoldenIR(t)
	results := core.BuildPCIDSS4Catalog(ir, core.Workload{})

	var storageEncryption, networkSecurity []core.ComplianceControlResult
	for _, r := range results {
		switch r.ControlID {
		case "pci_dss_4.req_3.storage_encryption":
			storageEncryption = append(storageEncryption, r)
		case "pci_dss_4.req_1.no_public_database_route":
			networkSecurity = append(networkSecurity, r)
		}
	}

	// golden/aws has exactly one real managed_database node (aws_db_instance.payments).
	if len(storageEncryption) != 1 {
		t.Fatalf("got %d storage-encryption results, want 1 (aws_db_instance.payments)", len(storageEncryption))
	}
	if storageEncryption[0].Result.Status != core.ComplianceSatisfied {
		t.Errorf("storage encryption Status = %q, want satisfied — golden/aws/rds.tf declares storage_encrypted = true", storageEncryption[0].Result.Status)
	}

	if len(networkSecurity) != 1 {
		t.Fatalf("got %d network-security results, want 1", len(networkSecurity))
	}
	// golden/aws's own RDS subnet ("data" tier) has no aws_route_table_association at
	// all (PC-125's own documented gap) — so this is honestly not_assessable, not a
	// fabricated satisfied/unsatisfied verdict.
	if networkSecurity[0].Result.Status != core.ComplianceNotAssessable {
		t.Errorf("network security Status = %q, want not_assessable — golden/aws's data subnets have no route table association (a real, documented, pre-existing gap)", networkSecurity[0].Result.Status)
	}
}

func TestSOC2_GoldenBundle_HandVerified(t *testing.T) {
	ir := realGoldenIR(t)
	results := core.BuildSOC2Catalog(ir, core.Workload{})

	var cc6 []core.ComplianceControlResult
	for _, r := range results {
		if r.ControlID == "soc2.cc6.no_public_database_route" {
			cc6 = append(cc6, r)
		}
	}
	if len(cc6) != 1 {
		t.Fatalf("got %d CC6 results, want 1", len(cc6))
	}
	if cc6[0].Classification != core.ClassificationPartial {
		t.Errorf("Classification = %q, want partially_assessable", cc6[0].Classification)
	}
	// Same real gap as the PCI DSS test above — not_assessable, never a fabricated
	// applicable/satisfied verdict.
	if cc6[0].Result.Status != core.ComplianceNotAssessable {
		t.Errorf("CC6 Status = %q, want not_assessable — golden/aws's data subnets have no route table association", cc6[0].Result.Status)
	}
}

func TestCISAWSCatalog_GoldenBundle_HandVerified(t *testing.T) {
	ir := realGoldenIR(t)
	results := core.BuildCISAWSCatalog(ir, core.Workload{})

	var storageEncryption, natRedundancy *core.ComplianceControlResult
	for i := range results {
		switch results[i].ControlID {
		case "cis_aws.storage_encryption":
			storageEncryption = &results[i]
		case "cis_aws.nat_gateway_redundancy":
			natRedundancy = &results[i]
		}
	}
	if storageEncryption == nil {
		t.Fatal("expected a cis_aws.storage_encryption result")
	}
	if storageEncryption.Result.Status != core.ComplianceSatisfied {
		t.Errorf("storage encryption Status = %q, want satisfied", storageEncryption.Result.Status)
	}
	if natRedundancy == nil {
		t.Fatal("expected a cis_aws.nat_gateway_redundancy result")
	}
	// golden/aws's real design: nat_a/nat_b/nat_c, one per public subnet — real
	// redundancy, hand-verified already by PC-28's own equivalent finding.
	if natRedundancy.Result.Status != core.ComplianceSatisfied {
		t.Errorf("NAT redundancy Status = %q, want satisfied — golden/aws has one NAT gateway per public subnet", natRedundancy.Result.Status)
	}
}
