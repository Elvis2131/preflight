package core_test

// PC-149: a subnet with no explicit NACL association is evaluated against its VPC's
// default NACL (AWS VPC User Guide, "Control subnet traffic with network access
// control lists": "If you don't explicitly associate a subnet with a network ACL, the
// subnet is automatically associated with the default network ACL"). The three rules,
// each pinned here, plus the two cases that must stay not_assessable.

import (
	"strings"
	"testing"

	"preflight/core"
)

// withoutNACLAssociations drops both subnets' NACL association edges from the fully
// flowing synthetic baseline, and puts both subnets in one VPC node.
func withoutNACLAssociations() *core.IR {
	ir := buildFlowTestIR(true, true)
	prov := syntheticProv()
	var edges []core.Edge
	for _, e := range ir.Edges {
		if e.To == "naclA" || e.To == "naclB" {
			continue
		}
		edges = append(edges, e)
	}
	ir.Nodes = append(ir.Nodes, rt("vpc", core.NodeTypeNetworkBoundary))
	edges = append(edges,
		core.Edge{ID: "v1", Type: core.EdgeTypeContainedIn, From: "subnetA", To: "vpc", Resolution: core.ResolutionKnown, Provenance: prov},
		core.Edge{ID: "v2", Type: core.EdgeTypeContainedIn, From: "subnetB", To: "vpc", Resolution: core.ResolutionKnown, Provenance: prov},
	)
	ir.Edges = edges
	return ir
}

func naclStep(tr core.Trace, name string) (core.TraceStep, bool) {
	for _, s := range tr.Steps {
		if s.Step == name {
			return s, true
		}
	}
	return core.TraceStep{}, false
}

// Rule 1: an explicit association still wins and is reported as explicit.
func TestResolveSubnetNACL_Explicit(t *testing.T) {
	ir := buildFlowTestIR(true, true)
	res, ok := core.ResolveSubnetNACL(ir.Nodes, ir.Edges, "subnetB")
	if !ok || res.Source != core.NACLExplicit || res.Profile.NACLID != "naclB" {
		t.Fatalf("got %+v ok=%v, want explicit naclB", res, ok)
	}
}

// Rule 3: nothing associated, nothing declared -> AWS's documented allow-all default,
// tagged assumed, and the trace step states the one real assumption.
func TestResolveSubnetNACL_AssumedDefault_TraceCarriesAssumedProvenanceAndReason(t *testing.T) {
	ir := withoutNACLAssociations()
	res, ok := core.ResolveSubnetNACL(ir.Nodes, ir.Edges, "subnetB")
	if !ok || res.Source != core.NACLAssumedDefault {
		t.Fatalf("got %+v ok=%v, want assumed_default", res, ok)
	}
	tr := core.BuildTrace(ir, "app", "db", "", "tcp", 5432)
	if !tr.Allowed {
		t.Fatalf("an allow-all default NACL must let the baseline through: %+v", tr.Steps)
	}
	for _, name := range []string{"nacl_source_egress", "nacl_dest_ingress"} {
		s, found := naclStep(tr, name)
		if !found {
			t.Fatalf("missing %s step", name)
		}
		if s.Provenance.Kind != core.KindAssumed || s.Provenance.Reason == "" {
			t.Errorf("%s provenance = %+v, want assumed with a reason", name, s.Provenance)
		}
		if !strings.Contains(s.Reason, "unmodified") {
			t.Errorf("%s reason %q must state the unmodified-default assumption", name, s.Reason)
		}
	}
	// A step that is not about the default NACL keeps derived provenance.
	if s, _ := naclStep(tr, "sg_dest_ingress"); s.Provenance.Kind != core.KindDerived {
		t.Errorf("sg_dest_ingress provenance = %q, want derived", s.Provenance.Kind)
	}
}

// Rule 2: the design declares the default NACL's rules -> those rules decide, and the
// step is NOT tagged assumed.
func TestResolveSubnetNACL_DeclaredDefault_RulesDecide(t *testing.T) {
	ir := withoutNACLAssociations()
	def := rt("defNacl", core.NodeTypeNetworkBoundary)
	def.RawAttributes = map[string]any{
		"default_nacl": true,
		"nacl_rules": []map[string]any{
			{"number": 100, "direction": "ingress", "protocol": "-1", "cidr": "0.0.0.0/0", "allow": true},
			// no egress allow: the declared default denies all outbound
		},
	}
	ir.Nodes = append(ir.Nodes, def)
	ir.Edges = append(ir.Edges, core.Edge{ID: "d1", Type: core.EdgeTypeContainedIn, From: "defNacl", To: "vpc", Resolution: core.ResolutionKnown, Provenance: syntheticProv()})

	res, ok := core.ResolveSubnetNACL(ir.Nodes, ir.Edges, "subnetA")
	if !ok || res.Source != core.NACLDeclaredDefault || res.Profile.NACLID != "defNacl" {
		t.Fatalf("got %+v ok=%v, want declared_default defNacl", res, ok)
	}
	tr := core.BuildTrace(ir, "app", "db", "", "tcp", 5432)
	if tr.Allowed {
		t.Fatal("the declared default has no egress allow, so the journey must be denied")
	}
	s, found := naclStep(tr, "nacl_source_egress")
	if !found || s.Decision != core.TraceDeny || s.Provenance.Kind == core.KindAssumed {
		t.Errorf("nacl_source_egress = %+v, want a derived deny from the declared default", s)
	}
}

// An association that cannot be ruled out stays not_assessable (I4): a dangling or
// unresolved depends_on edge might be the NACL association.
func TestResolveSubnetNACL_UnresolvedAssociation_NotAssessable(t *testing.T) {
	ir := withoutNACLAssociations()
	ir.Edges = append(ir.Edges, core.Edge{ID: "x", Type: core.EdgeTypeDependsOn, From: "subnetB", To: "no-such-node", Resolution: core.ResolutionUnresolved, Provenance: syntheticProv()})
	if _, ok := core.ResolveSubnetNACL(ir.Nodes, ir.Edges, "subnetB"); ok {
		t.Fatal("an unresolved association edge must not be treated as 'no association'")
	}
	tr := core.BuildTrace(ir, "app", "db", "", "tcp", 5432)
	if s, _ := naclStep(tr, "nacl_check"); s.Decision != core.TraceNotAssessable {
		t.Errorf("nacl_check = %+v, want not_assessable", s)
	}
}

// Default NACLs are declared but this subnet's VPC cannot be resolved to pick one.
func TestResolveSubnetNACL_DeclaredDefaultsButSubnetVPCUnknown_NotAssessable(t *testing.T) {
	ir := withoutNACLAssociations()
	var edges []core.Edge
	for _, e := range ir.Edges {
		if e.ID != "v2" {
			edges = append(edges, e)
		}
	}
	def := rt("defNacl", core.NodeTypeNetworkBoundary)
	def.RawAttributes = map[string]any{"default_nacl": true, "nacl_rules": []map[string]any{}}
	ir.Nodes = append(ir.Nodes, def)
	ir.Edges = append(edges, core.Edge{ID: "d1", Type: core.EdgeTypeContainedIn, From: "defNacl", To: "vpc", Resolution: core.ResolutionKnown, Provenance: syntheticProv()})
	if _, ok := core.ResolveSubnetNACL(ir.Nodes, ir.Edges, "subnetB"); ok {
		t.Fatal("with declared default NACLs and an unknown VPC, which default applies is unknown: must not assume allow-all")
	}
}
