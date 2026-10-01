package core_test

import (
	"strings"
	"testing"

	"preflight/core"
)

func netNode(id, service string) core.CanvasNode {
	return core.CanvasNode{ID: id, Type: "network_boundary", Label: id, Capability: map[string]string{}, ServiceID: service}
}

func TestValidateCanvasNetworkControls(t *testing.T) {
	rt := netNode("rt", "aws_route_table")
	rt.Routes = []core.CanvasRoute{
		{DestinationCIDR: "0.0.0.0/0", Target: "igw"},     // ok
		{DestinationCIDR: "10.1.0.0/16", Target: "nat"},   // ok
		{DestinationCIDR: "not-a-cidr", Target: "igw"},    // bad CIDR
		{DestinationCIDR: "10.2.0.0/16", Target: "vpc"},   // unmodelled target kind
		{DestinationCIDR: "10.3.0.0/16", Target: "ghost"}, // missing target
	}
	nacl := netNode("nacl", "aws_network_acl")
	nacl.NACLRules = []core.CanvasNACLRule{
		{Direction: "ingress", Number: 100, Protocol: "tcp", CIDRBlock: "10.0.0.0/8", Action: "allow"}, // ok
		{Direction: "ingress", Number: 110, Protocol: "tcp", CIDRBlock: "banana", Action: "deny"},      // bad CIDR
	}
	doc := core.CanvasDocument{Nodes: []core.CanvasNode{rt, nacl, netNode("igw", "aws_internet_gateway"), netNode("nat", "aws_nat_gateway"), netNode("vpc", "aws_vpc")}}

	vs := core.ValidateCanvasNetworkControls(doc)
	got := map[string]int{}
	for _, v := range vs {
		got[v.Rule+"@"+v.ResourceID]++
		if v.Message == "" || v.Source == "" {
			t.Errorf("violation %+v must carry a message and a source", v)
		}
	}
	want := map[string]int{core.NetCtlRouteCIDR + "@rt": 1, core.NetCtlRouteTarget + "@rt": 2, core.NetCtlNACLCIDR + "@nacl": 1}
	for k, n := range want {
		if got[k] != n {
			t.Errorf("%s: got %d violations, want %d (all: %+v)", k, got[k], n, vs)
		}
	}
	if len(vs) != 4 {
		t.Errorf("want exactly 4 violations, got %d: %+v", len(vs), vs)
	}
	for _, v := range vs {
		if v.Rule == core.NetCtlRouteTarget && !strings.Contains(v.Message, "never treated as a blackhole") && !strings.Contains(v.Message, "not a node") {
			t.Errorf("target violation should say why: %s", v.Message)
		}
	}
}

func TestValidateCanvasNetworkControls_ValidDocumentHasNone(t *testing.T) {
	rt := netNode("rt", "aws_route_table")
	rt.Routes = []core.CanvasRoute{{DestinationCIDR: "0.0.0.0/0", Target: "igw"}}
	if vs := core.ValidateCanvasNetworkControls(core.CanvasDocument{Nodes: []core.CanvasNode{rt, netNode("igw", "aws_internet_gateway")}}); len(vs) != 0 {
		t.Fatalf("a valid document must have no violations, got %+v", vs)
	}
}
