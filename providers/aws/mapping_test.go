package aws

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"
)

var golden8 = []string{
	"aws_route53_record",
	"aws_wafv2_web_acl",
	"aws_lb",
	"aws_eks_cluster",
	"aws_db_instance",
	"aws_elasticache_replication_group",
	"aws_sqs_queue",
	"aws_iam_role",
}

// TestAll8GoldenResourceTypesHaveMappings is PC-13's first acceptance criterion:
// "All 8 AWS resource types (Route53, WAF, ALB, EKS, RDS, ElastiCache, SQS, IAM) have
// mapping YAML."
func TestAll8GoldenResourceTypesHaveMappings(t *testing.T) {
	reg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	// >=, not ==: the registry may also carry edge-only mappings (e.g.
	// aws_wafv2_web_acl_association) alongside the 8 golden node-producing types —
	// see TestEdgeOnlyMappingsAreDistinctFromTheGolden8 below for that count.
	if len(reg) < len(golden8) {
		t.Errorf("Load() returned %d mappings, want at least %d (the golden 8) — got: %v", len(reg), len(golden8), reg)
	}
	for _, rt := range golden8 {
		m, ok := reg.Lookup(rt)
		if !ok {
			t.Errorf("no mapping loaded for golden resource type %q", rt)
			continue
		}
		if m.IsEdgeMapping() {
			t.Errorf("golden resource type %q is mapped as an edge, but all 8 golden types must produce a node", rt)
		}
	}
}

// TestEdgeOnlyMappingsAreDistinctFromTheGolden8 confirms the registry can hold
// non-node mappings without disturbing the golden-8 count or shape.
func TestEdgeOnlyMappingsAreDistinctFromTheGolden8(t *testing.T) {
	reg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	m, ok := reg.Lookup("aws_wafv2_web_acl_association")
	if !ok {
		t.Fatal("expected an edge mapping for aws_wafv2_web_acl_association")
	}
	if !m.IsEdgeMapping() {
		t.Fatal("aws_wafv2_web_acl_association should be an edge mapping, not a node mapping")
	}
	if m.Edge.FromAttribute == "" || m.Edge.ToAttribute == "" || m.Edge.Type == "" {
		t.Errorf("edge mapping is missing required fields: %+v", m.Edge)
	}
}

// TestEveryCapabilityFieldHasExplicitProvenance is PC-13's second acceptance
// criterion: "Every capability field in every mapping carries an explicit provenance
// tag." Load()'s own validate() already refuses to load a mapping missing this — this
// test additionally proves the refusal actually fires, by attempting to load a
// deliberately invalid mapping (not one of the real files) directly.
//
// The negative-proof half (validate() rejecting a mapping with no on_absent) moved to
// providers/mapping_test.go's TestValidate_RejectsCapabilityFieldWithNoOnAbsent (PC-22:
// validate() itself moved to the shared providers package, and is unexported there —
// this package can no longer call it directly). This test keeps the positive proof,
// which is genuinely AWS-specific (it inspects AWS's own real, loaded mapping data).
func TestEveryCapabilityFieldHasExplicitProvenance(t *testing.T) {
	reg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	for rt, m := range reg {
		for _, c := range m.Capabilities {
			if c.OnAbsent != OnAbsentNotAssessable && c.OnAbsent != OnAbsentAssumed {
				t.Errorf("%s field %s: OnAbsent is not a valid provenance tag: %q", rt, c.Field, c.OnAbsent)
			}
			if c.OnAbsent == OnAbsentAssumed && (c.AssumedDefault == "" || c.AssumedCitation == "") {
				t.Errorf("%s field %s: assumed without a default+citation is not a real provenance tag, it's an unattributed guess", rt, c.Field)
			}
		}
	}
}

// TestAddingA9thResourceTypeRequiresZeroAnalyseChanges is PC-13's third acceptance
// criterion — the structural half only now (PC-22): the dynamic half (a synthetic 9th
// mapping loads through the unmodified Registry type with zero special-casing) moved
// to providers/mapping_test.go's TestValidate_AcceptsA9thResourceTypeWithZeroSpecialCasing,
// since validate() itself moved to the shared providers package and is unexported
// there. This test keeps proving the other half: parse core/internal/analyse's own
// source and confirm it has no import of this package (or any providers/... package)
// at all — not "happens not to reference AWS today" but "is architecturally incapable
// of needing a change, because it never sees a resource type, only an already-mapped
// core.Node."
func TestAddingA9thResourceTypeRequiresZeroAnalyseChanges(t *testing.T) {
	t.Run("structural: core/internal/analyse imports no providers package", func(t *testing.T) {
		root, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		analyseDir := filepath.Join(root, "..", "..", "core", "internal", "analyse")

		fset := token.NewFileSet()
		pkgs, err := parser.ParseDir(fset, analyseDir, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parsing core/internal/analyse: %v", err)
		}
		for _, pkg := range pkgs {
			for _, file := range pkg.Files {
				for _, imp := range file.Imports {
					path := importPath(imp)
					if path == "preflight/providers/aws" || (len(path) >= len("preflight/providers") && path[:len("preflight/providers")] == "preflight/providers") {
						t.Errorf("core/internal/analyse imports %q — adding a resource type must never require touching analyse", path)
					}
				}
			}
		}
	})
}

func importPath(imp *ast.ImportSpec) string {
	// imp.Path.Value is a quoted string literal; strip the quotes.
	v := imp.Path.Value
	if len(v) >= 2 {
		return v[1 : len(v)-1]
	}
	return v
}
