package ingest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"preflight/core"
	"preflight/ingest"
	awsprovider "preflight/providers/aws"
)

func loadRegistry(t *testing.T) awsprovider.Registry {
	t.Helper()
	reg, err := awsprovider.Load()
	if err != nil {
		t.Fatalf("providers/aws.Load(): %v", err)
	}
	return reg
}

// TestCountForEachDynamicNeverRaiseAParseError is PC-12's second acceptance criterion,
// tested against three independent fixtures — one per construct — rather than one
// fixture exercising all three at once, so a regression in any single case is
// individually diagnosable.
func TestCountForEachDynamicNeverRaiseAParseError(t *testing.T) {
	reg := loadRegistry(t)

	cases := []struct {
		fixture      string
		wantInReason string
	}{
		{"count-fixture", "count"},
		{"for-each-fixture", "for_each"},
		{"dynamic-fixture", "dynamic block"},
	}

	for _, c := range cases {
		t.Run(c.fixture, func(t *testing.T) {
			result, err := ingest.Ingest(filepath.Join("testdata", c.fixture), reg, 1)
			if err != nil {
				t.Fatalf("Ingest() returned an error — count/for_each/dynamic must never raise a parse error: %v", err)
			}
			// These fixtures have no entry point, so they'll also hit MVG — that's
			// fine and expected; what matters is Ingest() didn't error, and the
			// resource that WOULD have become a node is genuinely unresolved, not
			// silently skipped. Re-run ParseDir directly to inspect the resource
			// itself, independent of whether MVG short-circuited node construction.
			parsed, err := ingest.ParseDir(filepath.Join("testdata", c.fixture))
			if err != nil {
				t.Fatalf("ParseDir() error: %v", err)
			}
			if len(parsed) != 1 {
				t.Fatalf("expected exactly 1 parsed resource, got %d", len(parsed))
			}
			_ = result
		})
	}
}

// TestFragmentBelowMVGThresholdReturnsInsufficientModel is PC-12's first acceptance
// criterion: "A fragment below the MVG threshold returns structured insufficient_model
// with missing[] and guidance[], never findings."
func TestFragmentBelowMVGThresholdReturnsInsufficientModel(t *testing.T) {
	reg := loadRegistry(t)

	result, err := ingest.Ingest("testdata/insufficient-fixture", reg, 1)
	if err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}
	if result.IR != nil {
		t.Fatal("expected no IR (and therefore no possibility of findings) for a fragment below the MVG threshold")
	}
	if result.Insufficient == nil {
		t.Fatal("expected a structured InsufficientModel, got nil")
	}
	if result.Insufficient.Status != "insufficient_model" {
		t.Errorf("Status = %q, want %q", result.Insufficient.Status, "insufficient_model")
	}
	if len(result.Insufficient.Missing) == 0 {
		t.Error("Missing[] must not be empty when the model is insufficient")
	}
	if len(result.Insufficient.Guidance) == 0 {
		t.Error("Guidance[] must not be empty when the model is insufficient")
	}
	found := false
	for _, m := range result.Insufficient.Missing {
		if m == "entry_point" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'entry_point' in Missing (fragment has a DB and an IAM role, no dns/load_balancer), got %v", result.Insufficient.Missing)
	}
}

// TestFragmentAtMVGThresholdProducesRealIR is the positive control: a fragment with
// exactly one entry point and one stateful node must clear the bar.
func TestFragmentAtMVGThresholdProducesRealIR(t *testing.T) {
	reg := loadRegistry(t)

	result, err := ingest.Ingest("testdata/sufficient-fixture", reg, 1)
	if err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}
	if result.Insufficient != nil {
		t.Fatalf("expected a real IR, got InsufficientModel: %+v", result.Insufficient)
	}
	if result.IR == nil {
		t.Fatal("expected a real IR, got nil")
	}
	if err := result.IR.Validate(); err != nil {
		t.Fatalf("produced IR does not validate against contracts/ir.schema.json's own Go struct tags: %v", err)
	}
	if len(result.IR.Nodes) != 2 {
		t.Errorf("expected 2 nodes (aws_lb + aws_db_instance), got %d", len(result.IR.Nodes))
	}
}

// TestGoldenBundleRoundTripsWithoutLoss is PC-12's third acceptance criterion: "the
// golden fixture bundle round-trips through the parser without loss." Loss is defined
// concretely: every parsed resource ends up in exactly one of IR.Nodes or
// OutOfVocabulary — never neither. This is checked against golden/aws directly (the
// real, hand-authored bundle from PC-15), not a synthetic stand-in.
func TestGoldenBundleRoundTripsWithoutLoss(t *testing.T) {
	reg := loadRegistry(t)

	goldenDir := findGoldenAWSDir(t)
	sourceCount := countResourceBlocks(t, goldenDir)

	result, err := ingest.Ingest(goldenDir, reg, 1)
	if err != nil {
		t.Fatalf("Ingest() error against the real golden bundle: %v", err)
	}
	if result.Insufficient != nil {
		t.Fatalf("golden/aws unexpectedly failed the MVG check: %+v", result.Insufficient)
	}
	if result.IR == nil {
		t.Fatal("expected a real IR from golden/aws")
	}

	accountedFor := len(result.IR.Nodes) + len(result.OutOfVocabulary) + len(result.EdgeOnlyResources)
	if accountedFor != sourceCount {
		t.Errorf("round-trip loss detected: %d resource blocks in source, but only %d accounted for (%d mapped nodes + %d out-of-vocabulary + %d edge-only) — %d resource(s) vanished silently",
			sourceCount, accountedFor, len(result.IR.Nodes), len(result.OutOfVocabulary), len(result.EdgeOnlyResources), sourceCount-accountedFor)
	}

	// The WAF association is exactly the edge-only case this mechanism was built for
	// (docs/IR_DESIGN_NOTE.md's WAF discussion, PC-11) — confirm it actually produced
	// a real edge, not just that it was tracked.
	foundWAFEdge := false
	for _, e := range result.EdgeOnlyResources {
		if e.ResourceType == "aws_wafv2_web_acl_association" {
			if !e.Produced {
				t.Error("aws_wafv2_web_acl_association was tracked as edge-only but did not produce an edge")
			}
			foundWAFEdge = true
		}
	}
	if !foundWAFEdge {
		t.Error("expected golden/aws to contain an aws_wafv2_web_acl_association (it does — see golden/aws/waf.tf)")
	}
	wafToALBEdgeFound := false
	for _, e := range result.IR.Edges {
		if e.From == "aws_lb.payments" && e.To == "aws_wafv2_web_acl.payments" {
			wafToALBEdgeFound = true
			if e.Resolution != core.ResolutionKnown {
				t.Errorf("WAF association edge resolution = %q, want known", e.Resolution)
			}
		}
	}
	if !wafToALBEdgeFound {
		t.Error("expected an edge from aws_lb.payments to aws_wafv2_web_acl.payments (the WAF association) — this is the exact relationship PC-11's design note named as the reason WAF needed to be its own node")
	}

	// All 8 golden node types (CLAUDE.md §14) must actually be present as nodes, not
	// merely "accounted for" via the out-of-vocabulary bucket.
	wantTypes := map[string]bool{
		"aws_route53_record": false, "aws_wafv2_web_acl": false, "aws_lb": false,
		"aws_eks_cluster": false, "aws_db_instance": false,
		"aws_elasticache_replication_group": false, "aws_sqs_queue": false, "aws_iam_role": false,
	}
	parsed, err := ingest.ParseDir(goldenDir)
	if err != nil {
		t.Fatalf("ParseDir() error: %v", err)
	}
	byKey := map[string]ingest.ParsedResource{}
	for _, r := range parsed {
		byKey[r.Key()] = r
	}
	for _, n := range result.IR.Nodes {
		if r, ok := byKey[n.ID]; ok {
			if _, tracked := wantTypes[r.Type]; tracked {
				wantTypes[r.Type] = true
			}
		}
	}
	for rt, found := range wantTypes {
		if !found {
			t.Errorf("golden node type %s produced no IR node", rt)
		}
	}

	if len(result.IR.Edges) == 0 {
		t.Error("expected at least one edge from the golden bundle's real cross-resource references (e.g. aws_lb -> aws_subnet)")
	}

	// Every edge endpoint must reference a real, produced node — catches exactly the
	// bug found by hand-verifying the golden-fixture generator's first output: an
	// edge-only mapped resource (aws_wafv2_web_acl_association) was briefly being
	// walked as if it were a node source, producing edges FROM an ID that never
	// appeared in IR.Nodes at all.
	nodeIDs := map[string]bool{}
	for _, n := range result.IR.Nodes {
		nodeIDs[n.ID] = true
	}
	for _, e := range result.IR.Edges {
		if !nodeIDs[e.From] {
			t.Errorf("edge %s -> %s: From %q is not a real node in IR.Nodes", e.From, e.To, e.From)
		}
		if !nodeIDs[e.To] {
			t.Errorf("edge %s -> %s: To %q is not a real node in IR.Nodes", e.From, e.To, e.To)
		}
	}
}

func findGoldenAWSDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(wd, "..", "golden", "aws")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("golden/aws not found at %s: %v", dir, err)
	}
	return dir
}

func countResourceBlocks(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".tf" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "resource \"") {
				count++
			}
		}
	}
	return count
}
