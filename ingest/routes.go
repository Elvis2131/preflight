// This file is PC-111: routes are structured data (destination CIDR -> target) a
// plain resource-reference edge cannot express, so they get dedicated construction
// here rather than going through providers.ResourceMapping's declarative system (which
// has no concept of "one resource produces N edges, each carrying different per-edge
// data" — see providers/aws/route_table.yaml's own doc comment).
//
// Two real Terraform shapes both produce routes, verified against golden/aws/
// network.tf (which uses the first) and the Terraform AWS provider's own aws_route
// resource docs (which the Card names explicitly, even though golden/aws doesn't use
// it): an aws_route_table's own inline `route { ... }` blocks, and a standalone
// aws_route resource naming its route_table_id. Both funnel into the same
// buildRouteEdges output.
package ingest

import (
	"fmt"
	"sort"

	"preflight/core"
	"preflight/providers"
)

// routeTargetAttribute names one real Terraform attribute that selects a route's
// target, and the AWS resource kind it points at. gateway_id is deliberately narrowed
// to internet-gateway targets only here: it is also used for a VPC's own local/virtual
// private gateway, but this project's v1 scope (matching golden/aws's own real usage
// and PC-111's own "add only fields the request engine and Failure Lab actually need")
// only builds real IGW routing — see the unsupported-target handling below for
// everything else.
var routeTargetAttribute = map[string]string{
	"gateway_id":     "internet_gateway",
	"nat_gateway_id": "nat_gateway",
}

// unsupportedRouteTargetAttribute names route-target attributes this ticket's own
// Card explicitly scopes out ("VPC endpoint / peering / TGW as not_assessable until
// modelled") — real, valid AWS route targets, just not ones this codebase has a node
// type or engine logic for yet. Never silently dropped (I4): buildRouteFindings below
// names each one.
var unsupportedRouteTargetAttribute = map[string]string{
	"vpc_peering_connection_id": "vpc_peering_connection",
	"transit_gateway_id":        "transit_gateway",
	"vpc_endpoint_id":           "vpc_endpoint",
	"egress_only_gateway_id":    "egress_only_internet_gateway",
	"local_gateway_id":          "local_gateway",
	"carrier_gateway_id":        "carrier_gateway",
	"core_network_arn":          "cloudwan_core_network",
}

// routeAttributes are the two real Terraform shapes' own attribute names for a
// route's destination — aws_route_table's inline route{} block uses cidr_block;
// the standalone aws_route resource uses destination_cidr_block (verified against
// the Terraform AWS provider's own aws_route resource schema).
const (
	inlineRouteCIDRAttr     = "cidr_block"
	standaloneRouteCIDRAttr = "destination_cidr_block"
)

// UnsupportedRoute names one route this codebase cannot yet model as a real edge —
// PC-111's own acceptance criterion: "unsupported route targets produce not_assessable
// naming the target," never treated as a blackhole or as success. Exported so
// Ingest's caller (server.Assess, cmd/gen-golden-fixtures) can turn each one into a
// real core.Finding via core.BuildUnsupportedRouteFindings — see that function's own
// doc comment for why this conversion happens outside ingest, not inside it.
type UnsupportedRoute struct {
	RouteTableKey string
	TargetKind    string
	Source        string
}

// buildRouteEdges constructs every real routes_to edge from both route shapes, and
// reports every unsupported-target route separately (never silently). It also reports
// which (fromKey, toKey) pairs it has fully accounted for, so the generic reference
// walker in buildEdges can skip them — without this, a route table's own
// gateway_id/nat_gateway_id reference would ALSO produce a second, plain depends_on
// edge with none of the real route data, double-modeling the same relationship.
func buildRouteEdges(parsed []ParsedResource, byKey map[string]ParsedResource, registry providers.Registry) (edges []core.Edge, unsupported []UnsupportedRoute, ownedRefs map[string]bool, producedFor map[string]bool) {
	ownedRefs = map[string]bool{}
	producedFor = map[string]bool{}

	for _, r := range parsed {
		switch r.Type {
		case "aws_route_table":
			for i, nb := range r.NestedBlocks["route"] {
				e, unsup, ok := buildOneRouteEdge(r.Key(), nb.Attributes, nb.References, inlineRouteCIDRAttr, i, byKey, registry, sourceRef(r))
				if unsup != nil {
					unsupported = append(unsupported, *unsup)
				}
				if ok {
					edges = append(edges, e)
					ownedRefs[r.Key()+"\x00"+e.To] = true
					producedFor[r.Key()] = true
				}
			}
		case "aws_route":
			rtRef, hasRT := r.AttributeReferences["route_table_id"]
			if !hasRT || len(rtRef) == 0 {
				continue // an aws_route with no resolvable route_table_id names nothing to attach to
			}
			routeTableKey := rtRef[0].Key()
			attrs := map[string]any{}
			for k, v := range r.Attributes {
				attrs[k] = v
			}
			singleRefs := map[string]ResourceRef{}
			for name, rs := range r.AttributeReferences {
				if len(rs) > 0 {
					singleRefs[name] = rs[0]
				}
			}
			e, unsup, ok := buildOneRouteEdge(routeTableKey, attrs, singleRefs, standaloneRouteCIDRAttr, 0, byKey, registry, sourceRef(r))
			if unsup != nil {
				unsupported = append(unsupported, *unsup)
			}
			if ok {
				edges = append(edges, e)
				ownedRefs[routeTableKey+"\x00"+e.To] = true
			}
			producedFor[r.Key()] = ok
		}
	}

	sort.Slice(edges, func(i, j int) bool { return edges[i].ID < edges[j].ID })
	return edges, unsupported, ownedRefs, producedFor
}

// buildOneRouteEdge turns one route's attributes (from either shape) into a real
// routes_to edge, or (for a target this codebase doesn't model) an unsupportedRoute
// report. ok is false when neither a supported nor an explicitly-unsupported target
// attribute was present at all (a route this parser genuinely cannot interpret,
// distinct from a recognized-but-unmodeled one).
func buildOneRouteEdge(routeTableKey string, attrs map[string]any, refs map[string]ResourceRef, cidrAttr string, index int, byKey map[string]ParsedResource, registry providers.Registry, source string) (core.Edge, *UnsupportedRoute, bool) {
	cidr, _ := attrs[cidrAttr].(string)

	for attr, kind := range unsupportedRouteTargetAttribute {
		if _, present := refs[attr]; present {
			return core.Edge{}, &UnsupportedRoute{RouteTableKey: routeTableKey, TargetKind: kind, Source: source}, false
		}
	}

	for attr, kind := range routeTargetAttribute {
		ref, present := refs[attr]
		if !present {
			continue
		}
		target, exists := byKey[ref.Key()]
		resolution := core.ResolutionKnown
		reason := ""
		if !exists {
			resolution = core.ResolutionUnresolved
			reason = "references " + ref.Key() + ", a declaration not present in the bundle (PRD §4)"
		} else if _, targetMapped := registry.Lookup(target.Type); !targetMapped {
			return core.Edge{}, nil, false
		}

		prov := core.NewProvenance(core.KindStated, source)
		if reason != "" {
			prov = prov.WithReason(reason)
		}
		return core.Edge{
			ID:         fmt.Sprintf("%s-route[%d]->%s", routeTableKey, index, ref.Key()),
			Type:       core.EdgeTypeRoutesTo,
			From:       routeTableKey,
			To:         ref.Key(),
			Resolution: resolution,
			Provenance: prov,
			RawAttributes: map[string]any{
				"destination_cidr": cidr,
				"target_kind":      kind,
			},
		}, nil, true
	}

	return core.Edge{}, nil, false
}
