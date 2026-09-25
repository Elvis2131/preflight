package analyse

import (
	"net/netip"
	"testing"
)

// TestLongestPrefixMatch_AWSDocsExample hand-verifies against the EXACT example table
// AWS's own docs use to illustrate this rule (route-tables-priority.html, "Longest
// prefix match" section): a route table with a peering-connection route more specific
// than its own default route to an IGW. AWS's own text: "Any traffic from the subnet
// that's destined for the 172.31.0.0/16 IP address range uses the peering connection,
// because this route is more specific than the route for internet gateway... All other
// traffic from the subnet uses the internet gateway."
func TestLongestPrefixMatch_AWSDocsExample(t *testing.T) {
	routes := []Route{
		{DestinationCIDR: "172.31.0.0/16", TargetNodeID: "pcx"},
		{DestinationCIDR: "0.0.0.0/0", TargetNodeID: "igw"},
	}

	inRange := netip.MustParseAddr("172.31.5.10")
	got, ok := LongestPrefixMatch(routes, inRange)
	if !ok || got.TargetNodeID != "pcx" {
		t.Fatalf("172.31.5.10: got %+v, ok=%v, want target=pcx (more specific route wins)", got, ok)
	}

	outOfRange := netip.MustParseAddr("8.8.8.8")
	got, ok = LongestPrefixMatch(routes, outOfRange)
	if !ok || got.TargetNodeID != "igw" {
		t.Fatalf("8.8.8.8: got %+v, ok=%v, want target=igw (falls through to the default route)", got, ok)
	}
}

func TestLongestPrefixMatch_NoRouteCovers_ReturnsNotOK(t *testing.T) {
	routes := []Route{{DestinationCIDR: "10.0.0.0/16", TargetNodeID: "local"}}
	_, ok := LongestPrefixMatch(routes, netip.MustParseAddr("8.8.8.8"))
	if ok {
		t.Fatal("expected ok=false when no declared route covers the destination")
	}
}

func TestLongestPrefixMatch_MoreSpecificSingleHostRoute(t *testing.T) {
	// Mirrors CLAUDE.md's own cited example convention (10.10.2.15/32 over 10.10.2.0/24).
	routes := []Route{
		{DestinationCIDR: "10.10.2.0/24", TargetNodeID: "broad"},
		{DestinationCIDR: "10.10.2.15/32", TargetNodeID: "specific"},
	}
	got, ok := LongestPrefixMatch(routes, netip.MustParseAddr("10.10.2.15"))
	if !ok || got.TargetNodeID != "specific" {
		t.Fatalf("got %+v, ok=%v, want target=specific (/32 beats /24)", got, ok)
	}
	got, ok = LongestPrefixMatch(routes, netip.MustParseAddr("10.10.2.20"))
	if !ok || got.TargetNodeID != "broad" {
		t.Fatalf("got %+v, ok=%v, want target=broad (/32 doesn't cover .20)", got, ok)
	}
}

func TestIsPublicSubnetByRoutes_AWSDocsExample(t *testing.T) {
	// Route table A from subnet-route-tables.html's own "Example 1": a default route
	// to an IGW, explicitly named as "what makes the subnet a public subnet."
	routes := []Route{
		{DestinationCIDR: "10.0.0.0/16", TargetNodeID: "local"},
		{DestinationCIDR: "0.0.0.0/0", TargetNodeID: "igw-1"},
	}
	if !IsPublicSubnetByRoutes(routes, map[string]bool{"igw-1": true}) {
		t.Fatal("a default route to a real IGW must be reported as a public subnet")
	}
}

func TestIsPublicSubnetByRoutes_PrivateSubnet_NoIGWRoute(t *testing.T) {
	// Route table B from the same AWS example: default route to a virtual private
	// gateway, no route to an IGW at all — "no route to the internet gateway, which is
	// what makes the subnet a VPN-only subnet," i.e. NOT public.
	routes := []Route{
		{DestinationCIDR: "10.0.0.0/16", TargetNodeID: "local"},
		{DestinationCIDR: "0.0.0.0/0", TargetNodeID: "vgw-1"},
	}
	if IsPublicSubnetByRoutes(routes, map[string]bool{"igw-1": true}) {
		t.Fatal("a default route to a virtual private gateway (not an IGW) must not be reported as public")
	}
}

func TestIsPublicSubnetByRoutes_NATRouteAlone_NotPublic(t *testing.T) {
	// golden/aws's own private subnets: default route to a NAT gateway, not an IGW —
	// outbound-only, the textbook "private subnet," never public.
	routes := []Route{{DestinationCIDR: "0.0.0.0/0", TargetNodeID: "nat-1"}}
	if IsPublicSubnetByRoutes(routes, map[string]bool{"igw-1": true}) {
		t.Fatal("a default route to a NAT gateway must not be reported as public")
	}
}
