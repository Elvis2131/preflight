// This file is PC-111's pure engine half: route selection and public-subnet
// derivation, both verified against real AWS VPC routing documentation before being
// implemented (not assumed from memory) — see each function's own citation.
//
// SCOPE, per this ticket's own "add only fields the request engine and Failure Lab
// actually need": the implicit "local" route every real route table carries (AWS VPC
// User Guide, subnet-route-tables.html: "Every route table contains a local route for
// communication within the VPC. This route is added by default to all route tables.")
// is NOT modeled here at all — it is a constant AWS guarantee, never removable and
// never a source of the two failure classes this ticket names (NAT loss, route
// removal), so there is nothing for this engine to evaluate about it. Only the
// EXPLICIT declared routes (default route to an IGW/NAT, and — PC-111's own scope
// exclusion — unsupported targets) are real, engine-relevant facts.
package analyse

import (
	"net/netip"
)

// Route is one route table's own declared route — provider-agnostic (no gateway_id/
// nat_gateway_id, just "this destination goes to this already-resolved node").
type Route struct {
	DestinationCIDR string
	TargetNodeID    string
}

// LongestPrefixMatch selects the route AWS itself would select for destIP, per AWS VPC
// User Guide, "How route priority works": "In general, we direct traffic using the
// most specific route that matches the traffic. This is known as the longest prefix
// match." (docs.aws.amazon.com/vpc/latest/userguide/route-tables-priority.html,
// verified 2026-09-25). Ties (two routes with the identical prefix length — invalid in
// real AWS, which rejects a route table with duplicate destinations, but this engine
// still needs a deterministic answer rather than an arbitrary one) are broken by
// sorted DestinationCIDR, matching this project's own established determinism
// discipline (Design §4.1: "sort before any traversal that feeds output ordering").
//
// ok is false when no declared route covers destIP at all — a real, honest "no route"
// answer (distinct from every candidate simply losing to a more specific one), which
// callers must not silently treat as either reachable or unreachable.
func LongestPrefixMatch(routes []Route, destIP netip.Addr) (Route, bool) {
	var best Route
	bestBits := -1
	found := false

	sorted := make([]Route, len(routes))
	copy(sorted, routes)
	sortRoutesByCIDR(sorted)

	for _, r := range sorted {
		prefix, err := netip.ParsePrefix(r.DestinationCIDR)
		if err != nil {
			continue // an unparsable CIDR is not this function's concern to reject; the
			// caller (core/routing.go) is responsible for surfacing a malformed route
			// as its own honest not_assessable fact, not silently skipping it here too
		}
		if !prefix.Contains(destIP) {
			continue
		}
		bits := prefix.Bits()
		if bits > bestBits {
			best = r
			bestBits = bits
			found = true
		}
	}
	return best, found
}

func sortRoutesByCIDR(routes []Route) {
	for i := 1; i < len(routes); i++ {
		for j := i; j > 0 && routes[j].DestinationCIDR < routes[j-1].DestinationCIDR; j-- {
			routes[j], routes[j-1] = routes[j-1], routes[j]
		}
	}
}

// IsPublicSubnetByRoutes reports whether a subnet's effective route table has a
// default-route (all-traffic) path to an internet gateway — the real, documented
// definition of "public subnet," per AWS VPC User Guide, "Subnet route tables":
// "Route table A is a custom route table that is explicitly associated with the
// public subnet. It has a route that sends all traffic to the internet gateway, WHICH
// IS WHAT MAKES THE SUBNET A PUBLIC SUBNET."
// (docs.aws.amazon.com/vpc/latest/userguide/subnet-route-tables.html, verified
// 2026-09-25) — not a label, a routing fact. "Default route" here means an IPv4
// all-traffic destination (0.0.0.0/0); this engine does not attempt IPv6 (::/0),
// unmodeled elsewhere in this codebase's IR.
func IsPublicSubnetByRoutes(routes []Route, igwTargetIDs map[string]bool) bool {
	const allIPv4 = "0.0.0.0/0"
	for _, r := range routes {
		if r.DestinationCIDR == allIPv4 && igwTargetIDs[r.TargetNodeID] {
			return true
		}
	}
	return false
}
