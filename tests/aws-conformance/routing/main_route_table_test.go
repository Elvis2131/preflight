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
