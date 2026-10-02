package core_test

// PC-151: a subnet with no explicit route table association is implicitly associated
// with the VPC's main route table, and every route table has a local route for the
// whole VPC (AWS VPC User Guide, "Subnet route tables"). Only that intra-VPC fact is
// decided; the step is tagged assumed because the local route can be replaced outside
// the design. Anything unresolved stays not_assessable (I4).

import (
	"strings"
	"testing"

	"preflight/core"
)

// implicitIR is two subnets in one VPC, each with an instance, SGs that permit the
// request, and NO route table, NACL or association of any kind.
func implicitIR() *core.IR {
	prov := syntheticProv()
	mk := func(id string, typ core.NodeType, raw map[string]any) core.Node {
		n := rt(id, typ)
		n.RawAttributes = raw
		return n
	}
	nodes := []core.Node{
		mk("vpc", core.NodeTypeNetworkBoundary, map[string]any{"network_role": "vpc", "cidr_block": "10.0.0.0/16"}),
		mk("subnetA", core.NodeTypeNetworkBoundary, map[string]any{"network_role": "subnet", "cidr_block": "10.0.1.0/24"}),
		mk("subnetB", core.NodeTypeNetworkBoundary, map[string]any{"network_role": "subnet", "cidr_block": "10.0.2.0/24"}),
		mk("app", core.NodeTypeCompute, map[string]any{"capability_level": string(core.CapabilityRequestSimulation)}),
		mk("db", core.NodeTypeManagedDatabase, map[string]any{"capability_level": string(core.CapabilityFailureSimulation)}),
		mk("sgApp", core.NodeTypeNetworkBoundary, map[string]any{"security_group_rules": []map[string]any{{"direction": "egress", "protocol": "-1", "cidr_blocks": []any{"0.0.0.0/0"}}}}),
		mk("sgDb", core.NodeTypeNetworkBoundary, map[string]any{"security_group_rules": []map[string]any{{"direction": "ingress", "protocol": "tcp", "from_port": 5432, "to_port": 5432, "cidr_blocks": []any{"10.0.1.0/24"}}}}),
	}
	e := func(id string, t core.EdgeType, from, to string) core.Edge {
		return core.Edge{ID: id, Type: t, From: from, To: to, Resolution: core.ResolutionKnown, Provenance: prov}
	}
	return &core.IR{SchemaVersion: "1.4.0", VersionNumber: 1, VersionHash: "h", Nodes: nodes, Edges: []core.Edge{
		e("1", core.EdgeTypeContainedIn, "app", "subnetA"), e("2", core.EdgeTypeContainedIn, "db", "subnetB"),
		e("3", core.EdgeTypeContainedIn, "subnetA", "vpc"), e("4", core.EdgeTypeContainedIn, "subnetB", "vpc"),
		e("5", core.EdgeTypeDependsOn, "app", "sgApp"), e("6", core.EdgeTypeDependsOn, "db", "sgDb"),
	}}
}

func routeStep(tr core.Trace) (core.TraceStep, bool) {
	for _, s := range tr.Steps {
		if s.Step == "route_selection" {
			return s, true
		}
	}
	return core.TraceStep{}, false
}

func TestImplicitMainRouteTable_IntraVPC_AllowedAndAssumed(t *testing.T) {
	tr := core.BuildTrace(implicitIR(), "app", "db", "", "tcp", 5432)
	if !tr.Allowed {
		t.Fatalf("two subnets in one VPC reach each other over the local route: %+v", tr.Steps)
	}
	s, ok := routeStep(tr)
	if !ok || s.Decision != core.TraceAllow || s.Provenance.Kind != core.KindAssumed || !strings.Contains(s.Reason, "unmodified") {
		t.Errorf("route_selection = %+v, want allow tagged assumed and stating the unmodified assumption", s)
	}
}

func TestImplicitMainRouteTable_DifferentVPCs_NotAssessable(t *testing.T) {
	ir := implicitIR()
	ir.Nodes = append(ir.Nodes, func() core.Node {
		n := rt("vpc2", core.NodeTypeNetworkBoundary)
		n.RawAttributes = map[string]any{"network_role": "vpc"}
		return n
	}())
	for i := range ir.Edges {
		if ir.Edges[i].ID == "4" {
			ir.Edges[i].To = "vpc2"
		}
	}
	tr := core.BuildTrace(ir, "app", "db", "", "tcp", 5432)
	if s, _ := routeStep(tr); s.Decision != core.TraceNotAssessable {
		t.Errorf("subnets in different VPCs: the local route does not connect them; got %+v", s)
	}
}

func TestImplicitMainRouteTable_VPCUnknown_NotAssessable(t *testing.T) {
	ir := implicitIR()
	var edges []core.Edge
	for _, e := range ir.Edges {
		if e.ID != "4" {
			edges = append(edges, e)
		}
	}
	ir.Edges = edges
	if s, _ := routeStep(core.BuildTrace(ir, "app", "db", "", "tcp", 5432)); s.Decision != core.TraceNotAssessable {
		t.Errorf("a subnet whose VPC is unknown must not be assumed to share one: %+v", s)
	}
}

// An association that exists but points at an empty route table, or one that cannot be
// resolved, is NOT "no association": the implicit rule must not fire.
func TestImplicitMainRouteTable_ExplicitOrUnresolvedAssociation_DoesNotFire(t *testing.T) {
	empty := implicitIR()
	rtNode := rt("rtB", core.NodeTypeNetworkBoundary)
	rtNode.RawAttributes = map[string]any{"network_role": "route_table"} // a table with no routes
	empty.Nodes = append(empty.Nodes, rtNode)
	empty.Edges = append(empty.Edges, core.Edge{ID: "x", Type: core.EdgeTypeDependsOn, From: "subnetB", To: "rtB", Resolution: core.ResolutionKnown, Provenance: syntheticProv()})
	if s, _ := routeStep(core.BuildTrace(empty, "app", "db", "", "tcp", 5432)); s.Decision != core.TraceNotAssessable {
		t.Errorf("explicit association to an empty route table must keep its old not_assessable behaviour: %+v", s)
	}

	dangling := implicitIR()
	dangling.Edges = append(dangling.Edges, core.Edge{ID: "y", Type: core.EdgeTypeDependsOn, From: "subnetB", To: "no-such", Resolution: core.ResolutionUnresolved, Provenance: syntheticProv()})
	if s, _ := routeStep(core.BuildTrace(dangling, "app", "db", "", "tcp", 5432)); s.Decision != core.TraceNotAssessable {
		t.Errorf("an unresolved association cannot be ruled out, so it is not 'implicit': %+v", s)
	}
}
