// Package routing holds PC-111's real conformance tests — route tables, IGW, NAT
// gateway. See tests/aws-conformance/harness for the format these tests use.
package routing

import (
	"net/netip"
	"testing"

	"preflight/core"
	"preflight/tests/aws-conformance/harness"
)

// TestRouteLPM_001_MoreSpecificRouteWins is ROUTE-LPM-001, the Card's own named
// example ID: real route selection uses longest-prefix match, verified against AWS's
// own worked example.
func TestRouteLPM_001_MoreSpecificRouteWins(t *testing.T) {
	spec := harness.Verify(t, harness.Spec{
		ID:            "ROUTE-LPM-001",
		Rule:          "When multiple routes could match a destination, AWS directs traffic using the most specific (longest-prefix) route, not the default route.",
		Source:        "https://docs.aws.amazon.com/vpc/latest/userguide/route-tables-priority.html",
		Scenario:      "A route table has both a default route (0.0.0.0/0 -> internet gateway) and a more specific route (172.31.0.0/16 -> VPC peering connection) — AWS's own documented example table.",
		Configuration: "routes = [{cidr: 172.31.0.0/16, target: pcx}, {cidr: 0.0.0.0/0, target: igw}]",
		Request:       "Resolve the route for destination 172.31.5.10.",
		Expected:      "The peering-connection route wins, because it is more specific than the default route to the internet gateway.",
	})

	routes := []core.Route{
		{DestinationCIDR: "172.31.0.0/16", TargetNodeID: "pcx"},
		{DestinationCIDR: "0.0.0.0/0", TargetNodeID: "igw"},
	}
	got, ok := core.LongestPrefixMatch(routes, netip.MustParseAddr("172.31.5.10"))
	if !ok || got.TargetNodeID != "pcx" {
		t.Fatalf("%s (%s): got %+v, ok=%v, want target=pcx — see %s", spec.ID, spec.Rule, got, ok, spec.Source)
	}
}

// TestRouteLPM_002_DefaultRouteHandlesEverythingElse is the complement: traffic NOT
// covered by the more specific route correctly falls through to the default route.
func TestRouteLPM_002_DefaultRouteHandlesEverythingElse(t *testing.T) {
	spec := harness.Verify(t, harness.Spec{
		ID:            "ROUTE-LPM-002",
		Rule:          "Traffic not covered by any more specific route falls through to the default route (0.0.0.0/0).",
		Source:        "https://docs.aws.amazon.com/vpc/latest/userguide/route-tables-priority.html",
		Scenario:      "The same route table as ROUTE-LPM-001.",
		Configuration: "routes = [{cidr: 172.31.0.0/16, target: pcx}, {cidr: 0.0.0.0/0, target: igw}]",
		Request:       "Resolve the route for destination 8.8.8.8 (outside 172.31.0.0/16).",
		Expected:      "The default route to the internet gateway wins — \"all other traffic from the subnet uses the internet gateway.\"",
	})

	routes := []core.Route{
		{DestinationCIDR: "172.31.0.0/16", TargetNodeID: "pcx"},
		{DestinationCIDR: "0.0.0.0/0", TargetNodeID: "igw"},
	}
	got, ok := core.LongestPrefixMatch(routes, netip.MustParseAddr("8.8.8.8"))
	if !ok || got.TargetNodeID != "igw" {
		t.Fatalf("%s (%s): got %+v, ok=%v, want target=igw — see %s", spec.ID, spec.Rule, got, ok, spec.Source)
	}
}

// TestSubnetAZ_001_DefaultRouteToIGWMakesSubnetPublic is SUBNET-AZ-001 (the Card's own
// named example ID, reused here since public-subnet derivation is this ticket's own
// scope, not a separate future placement engine's).
func TestSubnetAZ_001_DefaultRouteToIGWMakesSubnetPublic(t *testing.T) {
	spec := harness.Verify(t, harness.Spec{
		ID:            "SUBNET-AZ-001",
		Rule:          "A subnet is public because its route table has a default route to an internet gateway — a routing fact, not a label.",
		Source:        "https://docs.aws.amazon.com/vpc/latest/userguide/subnet-route-tables.html",
		Scenario:      "AWS's own documented Example 1: route table A, explicitly associated with a subnet, has a 0.0.0.0/0 route to an internet gateway.",
		Configuration: "routes = [{cidr: VPC-CIDR, target: local}, {cidr: 0.0.0.0/0, target: igw-id}]",
		Request:       "Evaluate whether this subnet is public.",
		Expected:      "True — AWS's own text: \"It has a route that sends all traffic to the internet gateway, which is what makes the subnet a public subnet.\"",
	})

	routes := []core.Route{
		{DestinationCIDR: "10.0.0.0/16", TargetNodeID: "local"},
		{DestinationCIDR: "0.0.0.0/0", TargetNodeID: "igw-id"},
	}
	if !core.IsPublicSubnetByRoutes(routes, map[string]bool{"igw-id": true}) {
		t.Fatalf("%s (%s): want true — see %s", spec.ID, spec.Rule, spec.Source)
	}
}

// TestSubnetAZ_002_NoIGWRouteMeansNotPublic is the complement, also from AWS's own
// same worked example (route table B, the VPN-only subnet).
func TestSubnetAZ_002_NoIGWRouteMeansNotPublic(t *testing.T) {
	spec := harness.Verify(t, harness.Spec{
		ID:            "SUBNET-AZ-002",
		Rule:          "A subnet whose route table has no default route to an internet gateway is not public, regardless of any label.",
		Source:        "https://docs.aws.amazon.com/vpc/latest/userguide/subnet-route-tables.html",
		Scenario:      "AWS's own documented Example 1: route table B (the main route table), implicitly associated with a subnet, routes 0.0.0.0/0 to a virtual private gateway, not an internet gateway.",
		Configuration: "routes = [{cidr: VPC-CIDR, target: local}, {cidr: 0.0.0.0/0, target: vgw-id}]",
		Request:       "Evaluate whether this subnet is public.",
		Expected:      "False — AWS's own text: \"no route to the internet gateway, which is what makes the subnet a VPN-only subnet.\"",
	})

	routes := []core.Route{
		{DestinationCIDR: "10.0.0.0/16", TargetNodeID: "local"},
		{DestinationCIDR: "0.0.0.0/0", TargetNodeID: "vgw-id"},
	}
	if core.IsPublicSubnetByRoutes(routes, map[string]bool{"igw-id": true}) {
		t.Fatalf("%s (%s): want false — see %s", spec.ID, spec.Rule, spec.Source)
	}
}
