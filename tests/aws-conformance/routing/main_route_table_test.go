package routing

import (
	"testing"

	"preflight/core"
	"preflight/tests/aws-conformance/harness"
)

func TestRouteMain_001_ImplicitAssociationAndLocalRoute(t *testing.T) {
	spec := harness.Verify(t, harness.Spec{
		ID:            "ROUTE-MAIN-001",
		Rule:          "A subnet with no explicit route table association is implicitly associated with the VPC's main route table, and every route table contains a local route for communication within the VPC.",
		Source:        "https://docs.aws.amazon.com/vpc/latest/userguide/subnet-route-tables.html",
		Scenario:      "Two subnets in one VPC with no route table of any kind declared, and security groups that permit the request.",
		Configuration: "subnetA and subnetB contained in one VPC; no route table, no association; the destination's SG admits tcp/5432 from subnetA",
		Request:       "Trace app (subnetA) -> db (subnetB) on tcp/5432.",
		Expected:      "route_selection allows over the local route, tagged assumed (the local route could have been replaced out of band); the trace reaches Allowed. A subnet in a DIFFERENT VPC is not reached by that local route.",
	})
	prov := core.NewProvenance(core.KindStated, "conformance")
	mk := func(id string, t core.NodeType, raw map[string]any) core.Node {
		return core.Node{ID: id, Type: t, Resolution: core.ResolutionKnown, Provenance: prov, RawAttributes: raw}
	}
	build := func(dbVPC string) *core.IR {
		nodes := []core.Node{
			mk("vpc", core.NodeTypeNetworkBoundary, map[string]any{"network_role": "vpc"}),
			mk("vpc2", core.NodeTypeNetworkBoundary, map[string]any{"network_role": "vpc"}),
			mk("sA", core.NodeTypeNetworkBoundary, map[string]any{"network_role": "subnet", "cidr_block": "10.0.1.0/24"}),
			mk("sB", core.NodeTypeNetworkBoundary, map[string]any{"network_role": "subnet", "cidr_block": "10.0.2.0/24"}),
			mk("app", core.NodeTypeCompute, map[string]any{"capability_level": string(core.CapabilityRequestSimulation)}),
			mk("db", core.NodeTypeManagedDatabase, map[string]any{"capability_level": string(core.CapabilityFailureSimulation)}),
			mk("sgA", core.NodeTypeNetworkBoundary, map[string]any{"security_group_rules": []map[string]any{{"direction": "egress", "protocol": "-1", "cidr_blocks": []any{"0.0.0.0/0"}}}}),
			mk("sgD", core.NodeTypeNetworkBoundary, map[string]any{"security_group_rules": []map[string]any{{"direction": "ingress", "protocol": "tcp", "from_port": 5432, "to_port": 5432, "cidr_blocks": []any{"10.0.1.0/24"}}}}),
		}
		e := func(id string, ty core.EdgeType, from, to string) core.Edge {
			return core.Edge{ID: id, Type: ty, From: from, To: to, Resolution: core.ResolutionKnown, Provenance: prov}
		}
		return &core.IR{SchemaVersion: "1.4.0", VersionNumber: 1, VersionHash: "h", Nodes: nodes, Edges: []core.Edge{
			e("1", core.EdgeTypeContainedIn, "app", "sA"), e("2", core.EdgeTypeContainedIn, "db", "sB"),
			e("3", core.EdgeTypeContainedIn, "sA", "vpc"), e("4", core.EdgeTypeContainedIn, "sB", dbVPC),
			e("5", core.EdgeTypeDependsOn, "app", "sgA"), e("6", core.EdgeTypeDependsOn, "db", "sgD"),
		}}
	}
	same := core.BuildTrace(build("vpc"), "app", "db", "", "tcp", 5432)
	if !same.Allowed {
		t.Fatalf("%s (%s): same-VPC subnets reach each other over the local route: %+v — see %s", spec.ID, spec.Rule, same.Steps, spec.Source)
	}
	for _, s := range same.Steps {
		if s.Step == "route_selection" && s.Provenance.Kind != core.KindAssumed {
			t.Errorf("%s: route_selection must be tagged assumed, got %q", spec.ID, s.Provenance.Kind)
		}
	}
	if core.BuildTrace(build("vpc2"), "app", "db", "", "tcp", 5432).Allowed {
		t.Errorf("%s: a subnet in a different VPC is not reached by the local route", spec.ID)
	}
}

func TestRouteMain_002_DeclaredMainTableDecidesEgress(t *testing.T) {
	spec := harness.Verify(t, harness.Spec{
		ID:            "ROUTE-MAIN-002",
		Rule:          "When a subnet has no explicit route table association, the VPC's main route table is used by default, and the main route table's routes can be added, removed and modified — so a declared main table's routes, not an assumed default, decide that subnet's internet egress.",
		Source:        "https://docs.aws.amazon.com/vpc/latest/userguide/subnet-route-tables.html",
		Scenario:      "A subnet with no association in a VPC whose main route table is declared with a default route to an internet gateway, and the same design with that table NOT declared as the main one.",
		Configuration: "subnet contained in the VPC; route table (declared main / not main) with 0.0.0.0/0 -> internet gateway",
		Request:       "Resolve the subnet's route table, classify it public/private, and evaluate its internet egress.",
		Expected:      "Declared main: the subnet uses that table, is public, and egress resolves to the internet gateway. Not declared: unresolved (not_assessable) — AWS guarantees only the local route, which says nothing about egress.",
	})
	prov := core.NewProvenance(core.KindStated, "conformance")
	build := func(declareMain bool) *core.IR {
		mk := func(id string, raw map[string]any) core.Node {
			return core.Node{ID: id, Type: core.NodeTypeNetworkBoundary, Resolution: core.ResolutionKnown, Provenance: prov, RawAttributes: raw}
		}
		e := func(id string, ty core.EdgeType, from, to string) core.Edge {
			return core.Edge{ID: id, Type: ty, From: from, To: to, Resolution: core.ResolutionKnown, Provenance: prov}
		}
		route := e("r", core.EdgeTypeRoutesTo, "mainrt", "igw")
		route.RawAttributes = map[string]any{"destination_cidr": "0.0.0.0/0", "target_kind": "internet_gateway"}
		return &core.IR{SchemaVersion: "1.4.0", VersionNumber: 1, VersionHash: "h",
			Nodes: []core.Node{
				mk("vpc", map[string]any{"network_role": "vpc"}), mk("subnet", map[string]any{"network_role": "subnet"}), mk("igw", nil),
				mk("mainrt", map[string]any{"network_role": "route_table", "main_route_table": declareMain}),
			},
			Edges: []core.Edge{e("1", core.EdgeTypeContainedIn, "subnet", "vpc"), e("2", core.EdgeTypeContainedIn, "mainrt", "vpc"), route}}
	}
	declared := build(true)
	if pub, has := core.IsPublicSubnetIR(declared, "subnet"); !has || !pub {
		t.Fatalf("%s (%s): declared main table must make the subnet public — see %s", spec.ID, spec.Rule, spec.Source)
	}
	if ok, kind, _ := core.EvaluateEgressRoute(declared, "subnet"); !ok || kind != "internet_gateway" {
		t.Errorf("%s: egress must resolve via the declared main table, got ok=%v kind=%q", spec.ID, ok, kind)
	}
	undeclared := build(false)
	if _, has := core.IsPublicSubnetIR(undeclared, "subnet"); has {
		t.Errorf("%s: without a declared main table public/private must stay unknown", spec.ID)
	}
	if ok, _, _ := core.EvaluateEgressRoute(undeclared, "subnet"); ok {
		t.Errorf("%s: without a declared main table egress must not be assumed", spec.ID)
	}
}
