package core_test

import (
	"reflect"
	"testing"

	"preflight/core"
)

func TestRegionOfAZ_OnlyTheDocumentedShape(t *testing.T) {
	good := map[string]string{"us-east-1a": "us-east-1", "eu-west-1b": "eu-west-1", "ap-southeast-2c": "ap-southeast-2", "us-gov-west-1a": "us-gov-west-1"}
	for az, want := range good {
		if got, ok := core.RegionOfAZ(az); !ok || got != want {
			t.Errorf("RegionOfAZ(%q) = %q, %v; want %q", az, got, ok, want)
		}
	}
	// Local Zone / Wavelength Zone / garbage: no region is guessed.
	for _, az := range []string{"", "us-west-2-lax-1", "us-west-2-lax-1a", "us-east-1-wl1-bos-wlz-1", "eu-west-1", "1a", "europe"} {
		if got, ok := core.RegionOfAZ(az); ok {
			t.Errorf("RegionOfAZ(%q) derived %q — a non-AZ code must derive nothing", az, got)
		}
	}
}

func TestDeriveSubnetFacts_FromTheTemplateShapedIR(t *testing.T) {
	ir := buildRouteIR()
	facts := core.DeriveSubnetFacts(ir)
	got := map[string]core.SubnetFact{}
	for _, f := range facts {
		got[f.NodeID] = f
	}
	if f := got["pub"]; f.Visibility != core.SubnetPublic || f.Region != "eu-west-1" || f.AvailabilityZone != "eu-west-1a" {
		t.Errorf("pub = %+v, want public in eu-west-1a / eu-west-1", f)
	}
	if f := got["priv"]; f.Visibility != core.SubnetPrivate || f.Region != "eu-west-1" {
		t.Errorf("priv = %+v, want private", f)
	}
	if f := got["orphan"]; f.Visibility != core.SubnetVisibilityUnknown || f.Reason == "" {
		t.Errorf("orphan (no route table) = %+v, want not_assessable with a reason — never defaulted to private", f)
	}
	if _, isSubnet := got["vpc"]; isSubnet {
		t.Errorf("a VPC is not a subnet: %+v", got["vpc"])
	}
	if !reflect.DeepEqual(facts, core.DeriveSubnetFacts(ir)) {
		t.Error("derivation must be deterministic")
	}
}

// buildRouteIR: a VPC; subnet "pub" (route table with a route to an IGW), subnet "priv"
// (route table with a route to a NAT), subnet "orphan" (no route table).
func buildRouteIR() *core.IR {
	prov := syntheticProv()
	node := func(id string, attrs map[string]any) core.Node {
		n := rt(id, core.NodeTypeNetworkBoundary)
		n.RawAttributes = attrs
		return n
	}
	nodes := []core.Node{
		node("vpc", map[string]any{"cidr_block": "10.0.0.0/16"}),
		node("pub", map[string]any{"cidr_block": "10.0.1.0/24", "availability_zone": "eu-west-1a"}),
		node("priv", map[string]any{"cidr_block": "10.0.2.0/24", "availability_zone": "eu-west-1b"}),
		node("orphan", map[string]any{"cidr_block": "10.0.3.0/24", "availability_zone": "eu-west-1c"}),
		node("rt-pub", nil), node("rt-priv", nil), node("igw", nil), node("nat", nil),
	}
	e := func(id string, t core.EdgeType, from, to string, raw map[string]any) core.Edge {
		return core.Edge{ID: id, Type: t, From: from, To: to, Resolution: core.ResolutionKnown, Provenance: prov, RawAttributes: raw}
	}
	return &core.IR{SchemaVersion: "1.4.0", VersionNumber: 1, VersionHash: "h", Nodes: nodes, Edges: []core.Edge{
		e("c1", core.EdgeTypeContainedIn, "pub", "vpc", nil), e("c2", core.EdgeTypeContainedIn, "priv", "vpc", nil), e("c3", core.EdgeTypeContainedIn, "orphan", "vpc", nil),
		e("a1", core.EdgeTypeDependsOn, "pub", "rt-pub", nil), e("a2", core.EdgeTypeDependsOn, "priv", "rt-priv", nil),
		e("r1", core.EdgeTypeRoutesTo, "rt-pub", "igw", map[string]any{"destination_cidr": "0.0.0.0/0", "target_kind": "internet_gateway"}),
		e("r2", core.EdgeTypeRoutesTo, "rt-priv", "nat", map[string]any{"destination_cidr": "0.0.0.0/0", "target_kind": "nat_gateway"}),
	}}
}
