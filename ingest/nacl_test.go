package ingest_test

// PC-113's own synthetic fixture (testdata/nacl-fixture): golden/aws has zero NACL
// resources, so this is this feature's only real ingest coverage — both real
// Terraform shapes (aws_network_acl's own inline ingress{} block, and the standalone
// aws_network_acl_rule resource).

import (
	"path/filepath"
	"testing"

	"preflight/core"
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

// PC-149: aws_default_network_acl is ingested as a node marked default_nacl, carries its
// inline rules, is contained_in its VPC — and core then resolves an unassociated subnet
// to it as the DECLARED default, not the assumed one.
func TestDefaultNetworkACL_IngestedAndResolvedAsDeclaredDefault(t *testing.T) {
	reg := loadRegistry(t)
	result, err := ingest.Ingest(filepath.Join("testdata", "default-nacl-fixture"), reg, 1)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if result.Insufficient != nil {
		t.Fatalf("got insufficient_model: %+v", result.Insufficient)
	}
	var def *core.Node
	for i := range result.IR.Nodes {
		if result.IR.Nodes[i].ID == "aws_default_network_acl.d" {
			def = &result.IR.Nodes[i]
		}
	}
	if def == nil {
		t.Fatal("aws_default_network_acl.d was not ingested as a node")
	}
	if d, _ := def.RawAttributes["default_nacl"].(bool); !d {
		t.Errorf("default_nacl marker = %v, want true", def.RawAttributes["default_nacl"])
	}
	if rules, _ := def.RawAttributes["nacl_rules"].([]map[string]any); len(rules) != 1 {
		t.Errorf("got %d nacl_rules, want the 1 inline ingress rule", len(rules))
	}
	res, ok := core.ResolveSubnetNACL(result.IR.Nodes, result.IR.Edges, "aws_subnet.s")
	if !ok || res.Source != core.NACLDeclaredDefault || res.Profile.NACLID != "aws_default_network_acl.d" {
		t.Fatalf("got %+v ok=%v, want the declared default NACL for the unassociated subnet", res, ok)
	}
}

// PC-151: both Terraform shapes that name a VPC's main route table are ingested and marked,
// the default table's inline routes become real routes, the association adds no duplicate
// containment edge, and core resolves an unassociated subnet to the declared table.
func TestMainRouteTable_BothShapes_IngestedAndResolved(t *testing.T) {
	reg := loadRegistry(t)
	result, err := ingest.Ingest(filepath.Join("testdata", "main-route-table-fixture"), reg, 1)
	if err != nil || result.IR == nil {
		t.Fatalf("Ingest: %v / %+v", err, result.Insufficient)
	}
	ir := result.IR
	mainOf := map[string]bool{}
	for _, n := range ir.Nodes {
		if n.RawAttributes["main_route_table"] == true {
			mainOf[n.ID] = true
		}
	}
	if !mainOf["aws_default_route_table.main"] || !mainOf["aws_route_table.custom_main"] || len(mainOf) != 2 {
		t.Fatalf("main route tables = %v, want exactly the default table and the one made main by association", mainOf)
	}
	dup := 0
	for _, e := range ir.Edges {
		if e.Type == core.EdgeTypeContainedIn && e.From == "aws_route_table.custom_main" && e.To == "aws_vpc.w" {
			dup++
		}
	}
	if dup != 1 {
		t.Errorf("got %d contained_in edges custom_main -> w, want 1 (de-duplicated)", dup)
	}
	if id, src, ok := core.ResolveSubnetRouteTable(ir, "aws_subnet.implicit"); !ok || id != "aws_default_route_table.main" || src != core.RouteTableDeclaredMain {
		t.Fatalf("implicit subnet resolved to %q %q ok=%v", id, src, ok)
	}
	if pub, has := core.IsPublicSubnetIR(ir, "aws_subnet.implicit"); !has || !pub {
		t.Errorf("the declared main table has a default route to an IGW, so the subnet is public: pub=%v has=%v", pub, has)
	}
	if id, _, ok := core.ResolveSubnetRouteTable(ir, "aws_subnet.other"); !ok || id != "aws_route_table.custom_main" {
		t.Errorf("the second VPC's subnet should use the table made main by association, got %q ok=%v", id, ok)
	}
	if pub, has := core.IsPublicSubnetIR(ir, "aws_subnet.other"); !has || pub {
		t.Errorf("that table has no routes: resolved and NOT public: pub=%v has=%v", pub, has)
	}
}
