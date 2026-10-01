package core

// PC-138/PC-139: server-side validation of canvas-authored network controls (route
// tables and NACLs). Like placement, the browser may highlight a problem but the server
// decides. Violations reuse PlacementViolation (rule, resource, message, source).
//
// What is checked is deliberately only what is either engine scope or plain
// well-formedness — NOT AWS behaviour this codebase could not cite: in particular no
// upper bound on a NACL rule number is enforced, because the AWS page consulted
// (vpc/latest/userguide/nacl-rules.html) states the evaluation order but no range.

import (
	"fmt"
	"net/netip"
	"sort"
)

const (
	NetCtlRouteTarget = "NETCTL-ROUTE-TARGET"
	NetCtlRouteCIDR   = "NETCTL-ROUTE-CIDR"
	NetCtlNACLCIDR    = "NETCTL-NACL-CIDR"

	engineScopeSource = "preflight: engine scope (PC-111 models only internet-gateway and NAT-gateway route targets)"
)

// routeTargetKind maps a route target node's service to the target_kind Terraform ingest
// stamps on a routes_to edge. ok is false for any target this engine does not model.
func routeTargetKind(serviceID string) (string, bool) {
	switch serviceID {
	case "aws_internet_gateway":
		return "internet_gateway", true
	case "aws_nat_gateway":
		return "nat_gateway", true
	}
	return "", false
}

// RouteTargetKind is the exported form, for ingest.
func RouteTargetKind(serviceID string) (string, bool) { return routeTargetKind(serviceID) }

// ValidateCanvasNetworkControls returns every problem with the routes and NACL rules in
// doc, sorted by rule then resource (NFR-1), or nil.
func ValidateCanvasNetworkControls(doc CanvasDocument) []PlacementViolation {
	byID := make(map[string]CanvasNode, len(doc.Nodes))
	for _, n := range doc.Nodes {
		byID[n.ID] = n
	}
	var out []PlacementViolation
	add := func(rule, id, msg, src string) {
		out = append(out, PlacementViolation{Rule: rule, ResourceID: id, Message: msg, Source: src})
	}
	for _, n := range doc.Nodes {
		for i, r := range n.Routes {
			if _, err := netip.ParsePrefix(r.DestinationCIDR); err != nil {
				add(NetCtlRouteCIDR, n.ID, fmt.Sprintf("route %d destination %q is not a valid CIDR", i+1, r.DestinationCIDR), "well-formedness")
			}
			target, ok := byID[r.Target]
			if !ok {
				add(NetCtlRouteTarget, n.ID, fmt.Sprintf("route %d targets %q, which is not a node in this design", i+1, r.Target), engineScopeSource)
				continue
			}
			if _, supported := routeTargetKind(target.ServiceID); !supported {
				add(NetCtlRouteTarget, n.ID, fmt.Sprintf("route %d targets %q, which is not an internet gateway or NAT gateway — other target kinds are not modelled and are never treated as a blackhole or as success", i+1, r.Target), engineScopeSource)
			}
		}
		for i, r := range n.NACLRules {
			if _, err := netip.ParsePrefix(r.CIDRBlock); err != nil {
				add(NetCtlNACLCIDR, n.ID, fmt.Sprintf("NACL rule %d (number %d) cidr_block %q is not a valid CIDR", i+1, r.Number, r.CIDRBlock), "well-formedness")
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Rule != out[j].Rule {
			return out[i].Rule < out[j].Rule
		}
		return out[i].ResourceID < out[j].ResourceID
	})
	return out
}
