package core_test

// PC-151: a subnet with no explicit route table association uses its VPC's main route table.
// When the design DECLARES that table (aws_default_route_table / aws_main_route_table_association)
// its routes decide egress and public/private for such subnets; when it does not, they stay
// not_assessable (I4) — AWS guarantees only the local route, which decides intra-VPC
// reachability but says nothing about internet egress.

import (
	"testing"

	"preflight/core"
)

func mainTableIR(declareMain bool) *core.IR {
	prov := syntheticProv()
	mk := func(id string, raw map[string]any) core.Node {
		n := rt(id, core.NodeTypeNetworkBoundary)
		n.RawAttributes = raw
		return n
	}
	nodes := []core.Node{
		mk("vpc", map[string]any{"network_role": "vpc"}),
		mk("subnet", map[string]any{"network_role": "subnet", "cidr_block": "10.0.1.0/24"}),
		mk("igw", nil),
		mk("mainrt", map[string]any{"network_role": "route_table", "main_route_table": declareMain}),
	}
	e := func(id string, t core.EdgeType, from, to string) core.Edge {
		return core.Edge{ID: id, Type: t, From: from, To: to, Resolution: core.ResolutionKnown, Provenance: prov}
	}
	route := e("r", core.EdgeTypeRoutesTo, "mainrt", "igw")
	route.RawAttributes = map[string]any{"destination_cidr": "0.0.0.0/0", "target_kind": "internet_gateway"}
	return &core.IR{SchemaVersion: "1.4.0", VersionNumber: 1, VersionHash: "h", Nodes: nodes, Edges: []core.Edge{
		e("1", core.EdgeTypeContainedIn, "subnet", "vpc"), e("2", core.EdgeTypeContainedIn, "mainrt", "vpc"), route,
	}}
}

func TestResolveSubnetRouteTable_DeclaredMain_DecidesPublicAndEgress(t *testing.T) {
	ir := mainTableIR(true)
	id, src, ok := core.ResolveSubnetRouteTable(ir, "subnet")
	if !ok || id != "mainrt" || src != core.RouteTableDeclaredMain {
		t.Fatalf("got %q %q ok=%v, want the declared main table", id, src, ok)
	}
	if pub, has := core.IsPublicSubnetIR(ir, "subnet"); !has || !pub {
		t.Errorf("a subnet using a declared main table with a default route to an IGW is public: pub=%v has=%v", pub, has)
	}
	if allowed, kind, _ := core.EvaluateEgressRoute(ir, "subnet"); !allowed || kind != "internet_gateway" {
		t.Errorf("egress must resolve through the declared main table: allowed=%v kind=%q", allowed, kind)
	}
}

// The same IR with the table NOT marked main: the subnet has no declared table, so nothing
// about its egress is known — never "therefore private" (I4).
func TestResolveSubnetRouteTable_NoDeclaredMain_StaysUnresolved(t *testing.T) {
	ir := mainTableIR(false)
	if _, _, ok := core.ResolveSubnetRouteTable(ir, "subnet"); ok {
		t.Fatal("an undeclared main table must not be assumed")
	}
	if _, has := core.IsPublicSubnetIR(ir, "subnet"); has {
		t.Error("public/private must stay unknown without a declared main table")
	}
	if allowed, _, reason := core.EvaluateEgressRoute(ir, "subnet"); allowed || reason == "" {
		t.Errorf("egress must be unresolved with a stated reason, got allowed=%v reason=%q", allowed, reason)
	}
}

func TestResolveSubnetRouteTable_AmbiguousOrExplicit_NotResolved(t *testing.T) {
	two := mainTableIR(true)
	second := rt("mainrt2", core.NodeTypeNetworkBoundary)
	second.RawAttributes = map[string]any{"network_role": "route_table", "main_route_table": true}
	two.Nodes = append(two.Nodes, second)
	two.Edges = append(two.Edges, core.Edge{ID: "x", Type: core.EdgeTypeContainedIn, From: "mainrt2", To: "vpc", Resolution: core.ResolutionKnown, Provenance: syntheticProv()})
	if _, _, ok := core.ResolveSubnetRouteTable(two, "subnet"); ok {
		t.Error("two declared main tables for one VPC is ambiguous: it must not pick one")
	}

	// An explicit association to a table that has no routes yet is NOT "no association".
	explicit := mainTableIR(true)
	emptyRT := rt("emptyrt", core.NodeTypeNetworkBoundary)
	emptyRT.RawAttributes = map[string]any{"network_role": "route_table"}
	explicit.Nodes = append(explicit.Nodes, emptyRT)
	explicit.Edges = append(explicit.Edges, core.Edge{ID: "y", Type: core.EdgeTypeDependsOn, From: "subnet", To: "emptyrt", Resolution: core.ResolutionKnown, Provenance: syntheticProv()})
	if id, _, ok := core.ResolveSubnetRouteTable(explicit, "subnet"); ok && id == "mainrt" {
		t.Error("a subnet with an explicit association must never fall back to the main table")
	}

	// VPC unknown: cannot tell which VPC's main table applies.
	noVPC := mainTableIR(true)
	var edges []core.Edge
	for _, e := range noVPC.Edges {
		if e.ID != "1" {
			edges = append(edges, e)
		}
	}
	noVPC.Edges = edges
	if _, _, ok := core.ResolveSubnetRouteTable(noVPC, "subnet"); ok {
		t.Error("a subnet whose VPC is unknown cannot be matched to a main table")
	}
}
