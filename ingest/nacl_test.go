package ingest_test

// PC-113's own synthetic fixture (testdata/nacl-fixture): golden/aws has zero NACL
// resources, so this is this feature's only real ingest coverage — both real
// Terraform shapes (aws_network_acl's own inline ingress{} block, and the standalone
// aws_network_acl_rule resource).

import (
	"path/filepath"
	"testing"

	"preflight/ingest"
)

func TestNACLRules_BothShapes(t *testing.T) {
	reg := loadRegistry(t)
	result, err := ingest.Ingest(filepath.Join("testdata", "nacl-fixture"), reg, 1)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if result.Insufficient != nil {
		t.Fatalf("got insufficient_model: %+v", result.Insufficient)
	}

	var rules []map[string]any
	for _, n := range result.IR.Nodes {
		if n.ID == "aws_network_acl.n" {
			rules, _ = n.RawAttributes["nacl_rules"].([]map[string]any)
		}
	}
	if len(rules) != 2 {
		t.Fatalf("got %d nacl_rules, want 2 (one inline ingress block + one standalone aws_network_acl_rule)", len(rules))
	}

	ruleNumber := func(r map[string]any) int {
		switch v := r["number"].(type) {
		case int:
			return v
		case float64:
			return int(v)
		default:
			return -999
		}
	}

	foundInline, foundStandalone := false, false
	for _, r := range rules {
		switch ruleNumber(r) {
		case 100:
			foundInline = true
			if r["allow"] != true {
				t.Errorf("rule 100: allow = %v, want true", r["allow"])
			}
		case 90:
			foundStandalone = true
			if r["allow"] != false {
				t.Errorf("rule 90: allow = %v, want false (rule_action=deny)", r["allow"])
			}
			if r["direction"] != "ingress" {
				t.Errorf("rule 90: direction = %v, want ingress (egress=false)", r["direction"])
			}
		}
	}
	if !foundInline {
		t.Error("expected the inline ingress{} block's rule (number 100)")
	}
	if !foundStandalone {
		t.Error("expected the standalone aws_network_acl_rule's rule (number 90)")
	}

	foundAssociation := false
	for _, e := range result.IR.Edges {
		if e.From == "aws_subnet.s" && e.To == "aws_network_acl.n" {
			foundAssociation = true
		}
	}
	if !foundAssociation {
		t.Error("expected a depends_on edge from the subnet to its associated NACL")
	}

	for _, oov := range result.OutOfVocabulary {
		if oov.ResourceType == "aws_network_acl_rule" {
			t.Error("aws_network_acl_rule must not be reported as out-of-vocabulary")
		}
	}
}
