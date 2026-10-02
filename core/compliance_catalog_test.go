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
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"preflight/core"
	"preflight/ingest"
	awsprovider "preflight/providers/aws"
)

func distinct(results []core.ComplianceControlResult) map[string]core.ComplianceControlResult {
	out := map[string]core.ComplianceControlResult{}
	for _, r := range results {
		if _, ok := out[r.ControlID]; !ok {
			out[r.ControlID] = r
		}
	}
	return out
}

// PCI DSS v4.0.1: one row per architecture-relevant sub-requirement (real v4.0.1 numbers, read from the
// standard) plus a "rest of Requirement N" row per principal requirement that has one, and a single row
// for each requirement with no architecture-evidenced sub-requirement. 19 rows: 1 assessable, 6 partial, 12 not.
func TestPCIDSS4Catalog_ClassifiedAtSubRequirementGranularity(t *testing.T) {
	results := core.BuildPCIDSS4Catalog(&core.IR{}, core.Workload{})
	counts := core.SummarizeComplianceCatalogCounts(results)
	if got := counts.Assessable + counts.Partial + counts.NotAssessable; got != 19 {
		t.Fatalf("got %d distinct controls, want 19", got)
	}
	if counts.Assessable != 1 || counts.Partial != 6 || counts.NotAssessable != 12 {
		t.Errorf("classes = %+v, want 1 assessable (1.4.4), 6 partial (1.3.1, 1.3.2, 3.5.1.2, 4.2.1, 6.4.2, 7.2.2), 12 not", counts)
	}
	byID := distinct(results)
	for id, class := range map[string]core.ControlClassification{
		"pci_dss_4:1.4.4":   core.ClassificationAssessable,
		"pci_dss_4:1.3.1":   core.ClassificationPartial,
		"pci_dss_4:1.3.2":   core.ClassificationPartial,
		"pci_dss_4:3.5.1.2": core.ClassificationPartial,
		"pci_dss_4:4.2.1":   core.ClassificationPartial,
		"pci_dss_4:6.4.2":   core.ClassificationPartial,
		"pci_dss_4:7.2.2":   core.ClassificationPartial,
		"pci_dss_4:1.rest":  core.ClassificationNotAssessable,
		"pci_dss_4:8":       core.ClassificationNotAssessable,
		"pci_dss_4:12":      core.ClassificationNotAssessable,
	} {
		if got, ok := byID[id]; !ok || got.Classification != class {
			t.Errorf("%s: %+v, want classification %s", id, got, class)
		}
	}
	// Every principal requirement 1..12 is represented, so the denominator stays honest.
	reqs := map[string]bool{}
	for _, r := range results {
		reqs[strings.SplitN(r.RequirementID, ".", 2)[0]] = true
	}
	for i := 1; i <= 12; i++ {
		if !reqs[strconv.Itoa(i)] {
			t.Errorf("principal requirement %d has no row", i)
		}
	}
}

// SOC 2: Security (CC1.1 to CC9.2, 33 criteria), Availability (A1.1 to A1.3) and Confidentiality (C1.1,
// C1.2) from the 2017 Trust Services Criteria. 38 rows: none assessable, 5 partial, 33 not.
func TestSOC2Catalog_ClassifiedPerCriterion(t *testing.T) {
	results := core.BuildSOC2Catalog(&core.IR{}, core.Workload{})
	counts := core.SummarizeComplianceCatalogCounts(results)
	if got := counts.Assessable + counts.Partial + counts.NotAssessable; got != 38 {
		t.Fatalf("got %d distinct controls, want 38", got)
	}
	if counts.Assessable != 0 || counts.Partial != 5 || counts.NotAssessable != 33 {
		t.Errorf("classes = %+v, want 0 assessable, 5 partial (CC6.1, CC6.3, CC6.6, CC6.7, A1.2), 33 not", counts)
	}
	byID := distinct(results)
	want := []string{}
	for cat, n := range map[string]int{"CC1": 5, "CC2": 3, "CC3": 4, "CC4": 2, "CC5": 3, "CC6": 8, "CC7": 5, "CC8": 1, "CC9": 2, "A1": 3, "C1": 2} {
		for i := 1; i <= n; i++ {
			want = append(want, "soc2:"+cat+"."+strconv.Itoa(i))
		}
	}
	for _, id := range want {
		if _, ok := byID[id]; !ok {
			t.Errorf("criterion %s has no row", id)
		}
	}
	for _, id := range []string{"soc2:CC6.1", "soc2:CC6.3", "soc2:CC6.6", "soc2:CC6.7", "soc2:A1.2"} {
		if byID[id].Classification != core.ClassificationPartial {
			t.Errorf("%s classification = %s, want partially_assessable", id, byID[id].Classification)
		}
	}
}

// Design §4.6's revisit trigger: "if the catalog approaches ~100 controls, reassess Go registry vs
// OPA/Rego." Recorded here so a future ticket trips this test before the catalog silently grows past it.
// Count at PC-121: 19 PCI DSS + 38 SOC 2 + 2 CIS AWS = 59 controls, of which only 13 carry evaluation logic
// (the rest are not_assessable rows with none), so the registry's cost is the logic, not the row count.
func TestComplianceCatalogs_TotalControlCount_UnderOPARevisitTrigger(t *testing.T) {
	ir := &core.IR{}
	all := append(core.BuildPCIDSS4Catalog(ir, core.Workload{}), core.BuildSOC2Catalog(ir, core.Workload{})...)
	all = append(all, core.BuildCISAWSCatalog(ir, core.Workload{})...)
	total := len(distinct(all))
	logic := 0
	for _, r := range distinct(all) {
		if r.Classification != core.ClassificationNotAssessable {
			logic++
		}
	}
	if total >= 100 {
		t.Errorf("total controls = %d: at Design §4.6's ~100 OPA/Rego revisit trigger; re-read that section before adding more", total)
	}
	t.Logf("controls: %d total, %d with evaluation logic (revisit trigger ~100)", total, logic)
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

// goldenBrokenIR ingests golden/aws-broken (the deliberately defective sibling bundle).
func goldenBrokenIR(t *testing.T) *core.IR {
	t.Helper()
	reg, err := awsprovider.Load()
	if err != nil {
		t.Fatal(err)
	}
	res, err := ingest.Ingest(filepath.Join("..", "golden", "aws-broken"), reg, 1)
	if err != nil || res.IR == nil {
		t.Fatalf("Ingest golden/aws-broken: %v", err)
	}
	return res.IR
}

type expectedResult struct {
	control, node string
	status        core.ComplianceStatus
}

func assertResults(t *testing.T, results []core.ComplianceControlResult, want []expectedResult) {
	t.Helper()
	got := map[string]core.ComplianceStatus{}
	for _, r := range results {
		got[r.ControlID+"|"+r.NodeID] = r.Result.Status
	}
	for _, w := range want {
		g, ok := got[w.control+"|"+w.node]
		if !ok {
			t.Errorf("no result for %s on %q", w.control, w.node)
			continue
		}
		if g != w.status {
			t.Errorf("%s on %q = %s, want %s", w.control, w.node, g, w.status)
		}
		delete(got, w.control+"|"+w.node)
	}
	// Anything not listed must be a control with no per-resource result of interest: a not_assessable row.
	for k, st := range got {
		for _, r := range results {
			if r.ControlID+"|"+r.NodeID == k && r.Classification != core.ClassificationNotAssessable {
				t.Errorf("unexpected, unlisted %s = %s: add it to the hand-verified table", k, st)
			}
		}
	}
}

// Hand-verified against golden/aws (decisions in the comments).
//   - 1.3.1 / 1.3.2: security.tf's database SG admits only the workload SG on 5432 and declares no egress, so
//     nothing is open to the world: supports the control (applicable, never satisfied: it needs documentation).
//   - 1.4.4 / CC6.6: golden's data-tier subnets have no route table association (the documented PC-125 gap), so
//     routing to the internet cannot be ruled out: not_assessable.
//   - 3.5.1.2 / CC6.1: rds.tf has storage_encrypted = true: supports, but disk-level encryption alone does not
//     satisfy PCI 3.5.1.2, so applicable.
//   - 6.4.2: alb.tf's load balancer is internet-facing and waf.tf associates a web ACL: applicable.
//   - 7.2.2 / CC6.3: eks_cluster and eks_node attach AWS-managed policies the engine cannot read: not_assessable.
//     payments_app's policy grants sqs and kms actions on specific in-bundle resources (symbolic references,
//     PC-159), so no statement is a wildcard: applicable (not satisfied: it is a partial control).
//   - 4.2.1 / CC6.7: TLS settings are not modelled: not_assessable.
//   - A1.2: rds.tf has multi_az = true (a synchronous standby): applicable.
func TestPCIDSSAndSOC2_GoldenBundle_HandVerified(t *testing.T) {
	ir := realGoldenIR(t)
	db, lb := "aws_db_instance.payments", "aws_lb.payments"
	roles := []string{"aws_iam_role.eks_cluster", "aws_iam_role.eks_node", "aws_iam_role.payments_app"}

	pci := []expectedResult{
		{"pci_dss_4:1.3.1", db, core.ComplianceApplicable},
		{"pci_dss_4:1.3.2", db, core.ComplianceApplicable},
		{"pci_dss_4:1.4.4", db, core.ComplianceNotAssessable},
		{"pci_dss_4:3.5.1.2", db, core.ComplianceApplicable},
		{"pci_dss_4:4.2.1", "", core.ComplianceNotAssessable},
		{"pci_dss_4:6.4.2", lb, core.ComplianceApplicable},
	}
	soc2 := []expectedResult{
		{"soc2:CC6.1", db, core.ComplianceApplicable},
		{"soc2:CC6.6", db, core.ComplianceNotAssessable},
		{"soc2:CC6.7", "", core.ComplianceNotAssessable},
		{"soc2:A1.2", db, core.ComplianceApplicable},
	}
	for _, r := range roles {
		want := core.ComplianceNotAssessable
		if r == "aws_iam_role.payments_app" {
			want = core.ComplianceApplicable
		}
		pci = append(pci, expectedResult{"pci_dss_4:7.2.2", r, want})
		soc2 = append(soc2, expectedResult{"soc2:CC6.3", r, want})
	}
	assertResults(t, core.BuildPCIDSS4Catalog(ir, core.Workload{}), pci)
	assertResults(t, core.BuildSOC2Catalog(ir, core.Workload{}), soc2)
}

// Hand-verified against golden/aws-broken, which is deliberately defective:
//   - 3.5.1.2 / CC6.1: storage_encrypted = false. Not a failure of these controls (account data can still be
//     protected inside the data, which the model cannot see) but not evidence either: not_assessable. The CIS
//     catalog judges the same fact on its own terms and reports it unsatisfied.
//   - 6.4.2: no web ACL attached: not_assessable (a firewall in front of the load balancer is not modelled).
//   - A1.2: multi_az = false: no standby declared: not_assessable, not a failure.
//   - 7.2.2 / CC6.3: payments_app's policy is Action * on Resource *, so unsatisfied (seeded defect 7, PC-157);
//     the two roles with AWS-managed attachments are not_assessable.
func TestPCIDSSAndSOC2_BrokenBundle_HandVerified(t *testing.T) {
	ir := goldenBrokenIR(t)
	db, lb := "aws_db_instance.payments", "aws_lb.payments"
	pci := []expectedResult{
		{"pci_dss_4:1.3.1", db, core.ComplianceApplicable},
		{"pci_dss_4:1.3.2", db, core.ComplianceApplicable},
		{"pci_dss_4:1.4.4", db, core.ComplianceNotAssessable},
		{"pci_dss_4:3.5.1.2", db, core.ComplianceNotAssessable},
		{"pci_dss_4:4.2.1", "", core.ComplianceNotAssessable},
		{"pci_dss_4:6.4.2", lb, core.ComplianceNotAssessable},
		{"pci_dss_4:7.2.2", "aws_iam_role.eks_cluster", core.ComplianceNotAssessable},
		{"pci_dss_4:7.2.2", "aws_iam_role.eks_node", core.ComplianceNotAssessable},
		{"pci_dss_4:7.2.2", "aws_iam_role.payments_app", core.ComplianceUnsatisfied},
	}
	soc2 := []expectedResult{
		{"soc2:CC6.1", db, core.ComplianceNotAssessable},
		{"soc2:CC6.3", "aws_iam_role.eks_cluster", core.ComplianceNotAssessable},
		{"soc2:CC6.3", "aws_iam_role.eks_node", core.ComplianceNotAssessable},
		{"soc2:CC6.3", "aws_iam_role.payments_app", core.ComplianceUnsatisfied},
		{"soc2:CC6.6", db, core.ComplianceNotAssessable},
		{"soc2:CC6.7", "", core.ComplianceNotAssessable},
		{"soc2:A1.2", db, core.ComplianceNotAssessable},
	}
	assertResults(t, core.BuildPCIDSS4Catalog(ir, core.Workload{}), pci)
	assertResults(t, core.BuildSOC2Catalog(ir, core.Workload{}), soc2)
}

// The Card's rule, on real bundles and not just an empty IR: a not-assessable-from-architecture control is
// never satisfied, and a partially assessable control never reports "satisfied" either: the most it can
// say is applicable (the architecture supports it), because the rest needs evidence a diagram cannot show.
func TestCatalogs_NeverReportSatisfiedBeyondWhatADiagramCanShow(t *testing.T) {
	for name, ir := range map[string]*core.IR{"aws": realGoldenIR(t), "aws-broken": goldenBrokenIR(t)} {
		all := append(core.BuildPCIDSS4Catalog(ir, core.Workload{}), core.BuildSOC2Catalog(ir, core.Workload{})...)
		for _, r := range all {
			switch r.Classification {
			case core.ClassificationNotAssessable:
				if r.Result.Status != core.ComplianceNotAssessable {
					t.Errorf("%s: %s is not_assessable_from_architecture but reports %s", name, r.ControlID, r.Result.Status)
				}
			case core.ClassificationPartial:
				if r.Result.Status == core.ComplianceSatisfied {
					t.Errorf("%s: %s is partially assessable but reports satisfied", name, r.ControlID)
				}
			}
		}
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
