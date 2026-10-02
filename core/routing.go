// This file is PC-111's core-level wrapper around core/internal/analyse's pure
// routing engine (routing.go): resolving real IR edges into the analyse.Route shape
// that engine needs, and attaching real Provenance to its answers — the same
// "internal/analyse stays pure and provider-agnostic; core/ wraps it with real IR
// data" split this project already uses throughout (ContainmentBlastRadius,
// DetectSPOFs, etc.).
package core

import (
	"fmt"
	"net/netip"

	"preflight/core/internal/analyse"
)

// Route is core's public mirror of core/internal/analyse.Route — I1 (core/internal/*
// is compiler-enforced private, unreachable from outside core/) means the internal
// engine's own Route type isn't visible to a caller like
// tests/aws-conformance/routing, which needs to construct routes directly to exercise
// LongestPrefixMatch/IsPublicSubnetByRoutes as conformance tests, not through a full
// IR/edges fixture every time. A type alias, not a redeclaration, so no conversion is
// needed at either boundary.
type Route = analyse.Route

// LongestPrefixMatch is the public entry point to PC-111's real-route-selection
// engine — see core/internal/analyse/routing.go's own doc comment for the full AWS
// citation and design reasoning; this is a direct, unwrapped passthrough (no
// additional IR resolution needed, unlike IsPublicSubnet below, since the caller
// already has the routes in hand).
func LongestPrefixMatch(routes []Route, destIP netip.Addr) (Route, bool) {
	return analyse.LongestPrefixMatch(routes, destIP)
}

// IsPublicSubnetByRoutes is the public entry point to the same-named internal
// function, for callers (conformance tests) that already have a route set in hand and
// don't need this package's own IR/edges resolution (IsPublicSubnet below).
func IsPublicSubnetByRoutes(routes []Route, igwTargetIDs map[string]bool) bool {
	return analyse.IsPublicSubnetByRoutes(routes, igwTargetIDs)
}

// routesByTable and igwTargetIDs are both derived purely from routes_to edges' own
// RawAttributes (destination_cidr, target_kind) — never from a provider-specific
// resource-type or ID-prefix check. This matters because NodeType alone cannot
// distinguish a route table from a VPC/subnet/IGW/NAT/SG: all five share the one
// canonical network_boundary type (PRD §4) — target_kind is real IR data ingest
// already attaches (ingest/routes.go), not something this function has to guess.
func routesByTable(edges []Edge) (routesByTableID map[string][]analyse.Route, igwTargetIDs map[string]bool) {
	routesByTableID = map[string][]analyse.Route{}
	igwTargetIDs = map[string]bool{}
	for _, e := range edges {
		if e.Type != EdgeTypeRoutesTo {
			continue
		}
		if cidr, ok := e.RawAttributes["destination_cidr"].(string); ok && cidr != "" {
			routesByTableID[e.From] = append(routesByTableID[e.From], analyse.Route{DestinationCIDR: cidr, TargetNodeID: e.To})
		}
		if kind, ok := e.RawAttributes["target_kind"].(string); ok && kind == "internet_gateway" {
			igwTargetIDs[e.To] = true
		}
	}
	return routesByTableID, igwTargetIDs
}

// EffectiveRouteTableID returns the route table ID a subnet actually uses, per AWS VPC
// User Guide, "Subnet route table association": "A subnet can only be associated with
// one route table at a time." (docs.aws.amazon.com/vpc/latest/userguide/
// subnet-route-tables.html, verified 2026-09-25) — found via the subnet's own
// depends_on edge (providers/aws/route_table_association.yaml) to a node that is
// itself the source of at least one routes_to edge, the purely structural definition
// of "is a route table" this function uses (see routesByTable's own doc comment for
// why NodeType alone can't answer this).
//
// ok is false when the subnet has no such edge — this function only answers for an
// EXPLICIT association. A subnet relying on the VPC's main route table is resolved by
// ResolveSubnetRouteTable (PC-151), which uses a DECLARED main table and otherwise stays
// honestly unresolved rather than assuming a default table.
func EffectiveRouteTableID(edges []Edge, subnetID string) (string, bool) {
	routeTableIDs := map[string]bool{}
	for _, e := range edges {
		if e.Type == EdgeTypeRoutesTo {
			routeTableIDs[e.From] = true
		}
	}
	for _, e := range edges {
		if e.Type == EdgeTypeDependsOn && e.From == subnetID && routeTableIDs[e.To] {
			return e.To, true
		}
	}
	return "", false
}

// UnsupportedRouteInfo is the plain-data mirror of ingest.UnsupportedRoute — core
// cannot import ingest (I1: core is the pure engine everything else depends ON, never
// the reverse), so the caller (server.Assess, cmd/gen-golden-fixtures) converts
// ingest.Result.UnsupportedRoutes into this shape before calling
// BuildUnsupportedRouteFindings below.
type UnsupportedRouteInfo struct {
	RouteTableID string
	TargetKind   string
	Source       string
}

// BuildUnsupportedRouteFindings is PC-111's own acceptance criterion, verbatim:
// "unsupported route targets produce not_assessable naming the target" — never
// treated as a blackhole (silently ignored) or as success (silently reachable). One
// Finding per unsupported route, always not_assessable, always naming both the target
// kind and the route table it was declared on.
func BuildUnsupportedRouteFindings(infos []UnsupportedRouteInfo) []Finding {
	findings := make([]Finding, 0, len(infos))
	for i, info := range infos {
		prov := NewProvenance(KindDerived, "ingest/routes:unsupported-target:"+info.RouteTableID).
			WithReason("route target kind \"" + info.TargetKind + "\" is not yet modelled (VPC peering / transit gateway / VPC endpoint scope, PC-111)")
		routeTableID := info.RouteTableID
		findings = append(findings, Finding{
			ID:    fmt.Sprintf("finding.routing.unsupported-target.%s.%d", routeTableID, i),
			Title: "Route to an unmodelled target kind (" + info.TargetKind + ")",
			Dimensions: FailureMode{
				Trigger:            "a route table declares a route to a target kind this engine does not yet evaluate",
				AffectedComponents: []string{routeTableID},
				Detection:          DetectionUnknown,
				Impact:             NotAssessable[any]("target kind \""+info.TargetKind+"\" is not modelled — impact cannot be derived", prov).ToEnvelope(),
				Likelihood:         DeriveLikelihood(prov).ToEnvelope(),
				Detectability:      DeriveDetectability(DetectionUnknown, prov).ToEnvelope(),
				Recoverability: Recoverability{
					FailoverPathExists: NotAssessable[any]("target kind not modelled", prov).ToEnvelope(),
					RPOFeasible:        NotAssessable[any]("target kind not modelled", prov).ToEnvelope(),
				},
			},
			Evidence: []EvidenceRef{
				{NodeID: &routeTableID, Description: "route declared with target kind \"" + info.TargetKind + "\" at " + info.Source},
			},
			Outcome: NotAssessable[any]("route target kind \""+info.TargetKind+"\" is not yet modelled by this engine", prov).ToEnvelope(),
		})
	}
	return findings
}

// IsPublicSubnet answers PC-111's own acceptance criterion directly against a real IR:
// "'Public subnet' derived from routes" — a thin wrapper resolving which route table a
// subnet actually uses (EffectiveRouteTableID) before asking
// analyse.IsPublicSubnetByRoutes the real routing question.
//
// hasRouteTable is false (and isPublic always false alongside it) when this subnet has
// no resolvable effective route table at all — an honest "don't know," never silently
// read as "therefore private."
func IsPublicSubnet(edges []Edge, subnetID string) (isPublic bool, hasRouteTable bool) {
	tableID, ok := EffectiveRouteTableID(edges, subnetID)
	if !ok {
		return false, false
	}
	routesByTableID, igwTargetIDs := routesByTable(edges)
	return analyse.IsPublicSubnetByRoutes(routesByTableID[tableID], igwTargetIDs), true
}

// ImplicitMainRouteTableReason states the one real assumption behind treating a subnet
// that has no explicit route table association as reaching its whole VPC (PC-151).
const ImplicitMainRouteTableReason = "AWS associates a subnet that has no explicit route table with the VPC's main route table, and every route table has a local route covering the VPC (VPC User Guide, \"Subnet route tables\"); ASSUMED unmodified — the local route's target can be replaced outside the design, and only the intra-VPC path is decided from it"

// containingVPC returns the node a subnet is contained_in and that carries the "vpc"
// network role, if exactly such a containment is known.
func containingVPC(ir *IR, subnetID string) (string, bool) {
	for _, e := range ir.Edges {
		if e.Type != EdgeTypeContainedIn || e.From != subnetID || e.Resolution != ResolutionKnown {
			continue
		}
		if n, ok := findNode(ir, e.To); ok && n.RawAttributes["network_role"] == "vpc" {
			return e.To, true
		}
	}
	return "", false
}

// hasExplicitRouteTableAssociation reports whether the subnet has ANY association that
// could be an explicit route table one: a depends_on edge to a node with the route_table
// role (even one with no routes yet), or one that is unresolved/dangling — an explicit
// association cannot be ruled out, so the implicit-main-table rule must not fire (I4).
func hasExplicitRouteTableAssociation(ir *IR, subnetID string) bool {
	for _, e := range ir.Edges {
		if e.Type != EdgeTypeDependsOn || e.From != subnetID {
			continue
		}
		n, ok := findNode(ir, e.To)
		if !ok || e.Resolution != ResolutionKnown || n.RawAttributes["network_role"] == "route_table" {
			return true
		}
	}
	return false
}

// RouteTableSource says how a subnet's effective route table was determined (PC-151).
type RouteTableSource string

const (
	// RouteTableExplicit: the subnet is associated with a route table in the design.
	RouteTableExplicit RouteTableSource = "explicit"
	// RouteTableDeclaredMain: no association, and the design declares the subnet's VPC main
	// route table (aws_default_route_table, or aws_main_route_table_association).
	RouteTableDeclaredMain RouteTableSource = "declared_main"
)

// ResolveSubnetRouteTable is the one place that answers "which route table does this subnet
// use?" for routing questions beyond the local route (internet egress, NAT, public/private):
//
//  1. an explicit association -> that table (EffectiveRouteTableID, unchanged);
//  2. none, and the design declares exactly one main route table for the subnet's VPC ->
//     that table (AWS: a subnet with no explicit association uses the main route table);
//  3. anything else -> ok=false, i.e. not_assessable. In particular a VPC whose main table
//     is NOT declared stays unresolved here: AWS guarantees only the local route, which
//     decides intra-VPC reachability (the trace's route_selection step) but says nothing
//     about internet egress, and an undeclared main table is not assumed to be one.
//
// ok is also false when an explicit association cannot be ruled out, when the subnet's VPC is
// unknown, or when more than one main route table is declared for the VPC (ambiguous).
func ResolveSubnetRouteTable(ir *IR, subnetID string) (tableID string, source RouteTableSource, ok bool) {
	if id, found := EffectiveRouteTableID(ir.Edges, subnetID); found {
		return id, RouteTableExplicit, true
	}
	if hasExplicitRouteTableAssociation(ir, subnetID) {
		return "", "", false
	}
	vpc, known := containingVPC(ir, subnetID)
	if !known {
		return "", "", false
	}
	var mains []string
	for _, n := range ir.Nodes {
		if n.RawAttributes["network_role"] != "route_table" || n.RawAttributes["main_route_table"] != true {
			continue
		}
		for _, e := range ir.Edges {
			if e.Type == EdgeTypeContainedIn && e.From == n.ID && e.To == vpc && e.Resolution == ResolutionKnown {
				mains = append(mains, n.ID)
				break
			}
		}
	}
	if len(mains) != 1 {
		return "", "", false
	}
	return mains[0], RouteTableDeclaredMain, true
}

// IsPublicSubnetIR is IsPublicSubnet over a whole IR, so a subnet that relies on a declared
// main route table is classified from that table's routes (PC-151).
func IsPublicSubnetIR(ir *IR, subnetID string) (isPublic bool, hasRouteTable bool) {
	tableID, _, ok := ResolveSubnetRouteTable(ir, subnetID)
	if !ok {
		return false, false
	}
	routesByTableID, igwTargetIDs := routesByTable(ir.Edges)
	return analyse.IsPublicSubnetByRoutes(routesByTableID[tableID], igwTargetIDs), true
}
