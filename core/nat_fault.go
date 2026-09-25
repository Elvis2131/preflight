// This file is PC-129: NAT gateway loss and route-removal faults, plus the
// single-NAT-multi-AZ SPOF finding PC-78 left as an open modelling question.
//
// DESIGN, resolving a real, necessary gap surfaced while building this ticket:
// BuildTrace's own route_selection step (core/trace.go, PC-114) never actually
// verified a route to the destination exists — it only checked that a route table
// was associated at all, always allowing cross-subnet traffic once one was. That is
// silently correct for intra-VPC traffic (AWS's own real, unremovable "local" route
// covers a VPC's entire CIDR range for every subnet inside it — never explicitly
// modelled as a routes_to edge, ingest only captures EXPLICIT routes) — which is
// exactly why "private → RDS still succeeds" was already true and needed no fix. It
// is NOT correct for egress-to-the-internet, which genuinely depends on a real,
// removable 0.0.0.0/0 route — BuildTrace has no way to represent "destination is the
// internet" at all (its own signature only takes a real destination node), so this
// ticket adds a narrow, dedicated evaluator for exactly that question rather than
// forcing it through BuildTrace's resource-to-resource shape.
//
// Faults are modelled as IR MUTATIONS (remove the specific routes_to edge(s)), per
// the Card's own instruction — "runs the request engine and structural flow AFTER
// mutating the model; never marks nodes failed by association." Nothing else in the
// IR is touched; a killed route's own route table, and every other route on it,
// remain exactly as declared.
package core

import (
	"fmt"
	"net/netip"
)

// EvaluateEgressRoute answers "can this subnet reach the internet" — a real
// LongestPrefixMatch (PC-111) against 0.0.0.0/0, using the subnet's own effective
// route table, checking that the matched route's target node still exists in ir
// (never assumed reachable just because the edge exists — a caller testing a NAT-loss
// fault passes an IR with that edge already removed, WithRouteRemoved/
// WithNATGatewayLost below).
func EvaluateEgressRoute(ir *IR, subnetID string) (allowed bool, targetKind string, reason string) {
	routeTableID, ok := EffectiveRouteTableID(ir.Edges, subnetID)
	if !ok {
		return false, "", "subnet has no resolvable effective route table"
	}

	routesByTableID, _ := routesByTable(ir.Edges)
	routes := routesByTableID[routeTableID]

	anyAddr := netip.MustParseAddr("0.0.0.0")
	match, ok := LongestPrefixMatch(routes, anyAddr)
	if !ok {
		return false, "", "no default route (0.0.0.0/0) exists on route table " + routeTableID
	}

	if !nodeExists(ir, match.TargetNodeID) {
		return false, "", "the default route's target (" + match.TargetNodeID + ") no longer exists — egress is broken"
	}

	kind := ""
	for _, e := range ir.Edges {
		if e.Type == EdgeTypeRoutesTo && e.From == routeTableID && e.To == match.TargetNodeID {
			kind, _ = e.RawAttributes["target_kind"].(string)
		}
	}
	return true, kind, "default route resolves to a reachable " + kind
}

// removedRouteTargetPrefix marks a route whose target has been faulted away — the
// edge itself is kept (a route table's structural identity, EffectiveRouteTableID, is
// "a node with at least one outgoing routes_to edge"; deleting a route table's own
// LAST edge would make it stop being recognized as a route table at all, a real
// artifact this codebase's own structural-identity convention exposed while building
// this fault). Retargeting to a sentinel ID guaranteed absent from ir.Nodes is also
// more accurate to real AWS behavior: deleting a NAT gateway does not delete the
// route entries that pointed at it — they simply become blackhole routes.
const removedRouteTargetPrefix = "!!removed-by-fault!!:"

// WithRouteRemoved returns a COPY of ir with every routes_to edge from routeTableID to
// destinationCIDR retargeted to a sentinel that resolves to nothing — never mutates
// the caller's own IR. This is PC-129's "route_removal" fault: exactly the traffic
// that relied on that specific route breaks; everything else on the same route table
// is untouched.
func WithRouteRemoved(ir *IR, routeTableID, destinationCIDR string) *IR {
	out := &IR{SchemaVersion: ir.SchemaVersion, VersionNumber: ir.VersionNumber, VersionHash: ir.VersionHash, Nodes: ir.Nodes}
	for _, e := range ir.Edges {
		if e.Type == EdgeTypeRoutesTo && e.From == routeTableID {
			if cidr, _ := e.RawAttributes["destination_cidr"].(string); cidr == destinationCIDR {
				e.To = removedRouteTargetPrefix + e.To
			}
		}
		out.Edges = append(out.Edges, e)
	}
	return out
}

// WithNATGatewayLost returns a COPY of ir with every routes_to edge TARGETING
// natGatewayID retargeted to a sentinel that resolves to nothing — PC-129's
// "nat_gateway_loss" fault. The NAT gateway node itself is left in place (it still
// structurally exists; it is simply no longer a usable route target) — never marks
// any OTHER node failed by association, per the Card's own instruction.
func WithNATGatewayLost(ir *IR, natGatewayID string) *IR {
	out := &IR{SchemaVersion: ir.SchemaVersion, VersionNumber: ir.VersionNumber, VersionHash: ir.VersionHash, Nodes: ir.Nodes}
	for _, e := range ir.Edges {
		if e.Type == EdgeTypeRoutesTo && e.To == natGatewayID {
			e.To = removedRouteTargetPrefix + e.To
		}
		out.Edges = append(out.Edges, e)
	}
	return out
}

// BuildNATSharedAcrossAZsFindings is PC-78's own open NAT-modelling question,
// resolved: a design where more than one private route table's own default route
// points at the SAME NAT gateway has a real, cross-AZ single point of failure —
// losing that one NAT gateway (or its AZ) breaks egress for every subnet that shares
// it. A route table whose default route points at its OWN distinct NAT gateway (the
// golden bundle's own real design: nat_a/nat_b/nat_c, one per AZ) produces no finding
// at all for that route table.
func BuildNATSharedAcrossAZsFindings(ir *IR) []Finding {
	routeTablesByNAT := map[string][]string{}
	for _, e := range ir.Edges {
		if e.Type != EdgeTypeRoutesTo {
			continue
		}
		kind, _ := e.RawAttributes["target_kind"].(string)
		if kind != "nat_gateway" {
			continue
		}
		routeTablesByNAT[e.To] = append(routeTablesByNAT[e.To], e.From)
	}

	var findings []Finding
	for natID, routeTables := range routeTablesByNAT {
		if len(routeTables) < 2 {
			continue // one route table per NAT gateway — the good, no-SPOF design
		}
		prov := NewProvenance(KindDerived, "core/nat_fault:shared-nat-spof:"+natID)
		findings = append(findings, buildSharedNATFinding(natID, routeTables, prov))
	}
	return findings
}

func buildSharedNATFinding(natID string, routeTables []string, prov Provenance) Finding {
	reason := fmt.Sprintf("NAT gateway %s is the default-route target for %d route tables (%v) — a single point of failure for every subnet that shares it", natID, len(routeTables), routeTables)
	return Finding{
		ID:    "finding.routing.nat-shared-across-azs." + natID,
		Title: "NAT gateway shared across multiple route tables/AZs (single point of failure)",
		Dimensions: FailureMode{
			Trigger:            "the shared NAT gateway (or its own AZ) is lost",
			AffectedComponents: routeTables,
			Detection:          DetectionModeled,
			Impact:             NotAssessable[any]("impact dimension not evaluated by this structural check", prov).ToEnvelope(),
			Likelihood:         DeriveLikelihood(prov).ToEnvelope(),
			Detectability:      DeriveDetectability(DetectionModeled, prov).ToEnvelope(),
			Recoverability: Recoverability{
				FailoverPathExists: Assessed[any](false, prov).ToEnvelope(),
				RPOFeasible:        NotAssessable[any]("RPO is not a meaningful concept for a NAT gateway (egress-only, no data of its own)", prov).ToEnvelope(),
			},
		},
		Evidence: []EvidenceRef{
			{NodeID: &natID, Description: reason},
		},
		Outcome: Assessed[any]("unsatisfied", prov).ToEnvelope(),
	}
}
