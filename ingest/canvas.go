// This file is PC-86's core: IngestCanvas is a second IR producer, symmetric to
// Ingest (HCL/Terraform) — both feed the exact same downstream pipeline (MVG check,
// core/analyse, core/simulate) and produce the identical IR shape. No canvas-specific
// field or shortcut exists anywhere in core/ — everything canvas-specific stays in
// this one file, on this side of the ingestion boundary, per this ticket's own
// acceptance criterion verbatim.
package ingest

import (
	"fmt"
	"strconv"

	"preflight/core"
	"preflight/providers"
)

// IngestCanvas builds an IR directly from a canvas-authored core.CanvasDocument
// (PC-85's own wire contract, frozen as contracts/canvas.schema.json). Unlike Ingest,
// a canvas node already declares its canonical NodeType directly — the palette IS the
// golden vocabulary (PC-85) — so there is no NodeType/edge-shape mapping to do
// through a providers.Registry the way an HCL resource has.
//
// PC-136 correction to this file's own earlier claim: a canvas node's NodeType alone
// does not resolve a PC-107 capability_level ("compute" cannot distinguish EC2 from
// Lambda). registry IS used here, for exactly that one purpose — resolving
// CanvasNode.ServiceID through the identical providers.Registry.Lookup the Terraform
// path already uses (see resolveCanvasCapabilityLevel below) — never a second lookup
// implementation, never a NodeType/edge-shape decision (those stay canvas-native, as
// this file's own original reasoning above still holds).
//
// Incompleteness handling is this function's actual point, not an afterthought (this
// ticket's own Conversation: "a half-drawn diagram is the steady state of using a
// canvas ... not a rare partial input"):
//   - A node is always core.ResolutionKnown — a drawn node is, by construction, fully
//     declared; there is no "the canvas referenced a node it forgot to draw" case at
//     the node level, only at the edge level.
//   - An edge whose From or To references a node ID absent from this same document
//     is still built (never dropped, never an error) but marked
//     core.ResolutionUnresolved with a stated reason — mirroring exactly how Ingest's
//     own buildEdges treats a reference to a declaration "not present in the bundle"
//     (see that function's own doc comment). Every downstream analysis function
//     already refuses to read through an unresolved edge without going through
//     AssessNode/AssessEdge's own resolution-state gate (PC-11) — so a dangling
//     canvas edge produces not_assessable results structurally, the same guarantee
//     HCL ingestion already has, not a new mechanism invented here.
//   - A node's capability values come directly from CanvasNode.Capability's own
//     string keys, reusing assignCapabilityField unchanged: canvas capability keys
//     are canonical field names (e.g. "encryption_mechanism"), not raw
//     provider-attribute names needing a providers.Registry translation, and
//     assignCapabilityField already recognizes canonical names directly (the same
//     switch cases an HCL mapping's own `field:` entries hit). A node with no
//     capability entered at all — or missing the one field a specific check reads —
//     produces a nil CapabilityModel field, and every compliance/failover check
//     already treats that as not_assessable, never a fabricated default.
func IngestCanvas(doc core.CanvasDocument, registry providers.Registry, versionNumber int) (Result, error) {
	nodeExists := make(map[string]bool, len(doc.Nodes))
	for _, n := range doc.Nodes {
		nodeExists[n.ID] = true
	}

	prov := core.NewProvenance(core.KindStated, "canvas")

	nodes := make([]core.Node, 0, len(doc.Nodes))
	for _, n := range doc.Nodes {
		nodes = append(nodes, core.Node{
			ID:         n.ID,
			Type:       n.Type,
			Resolution: core.ResolutionKnown,
			Capability: buildCanvasCapability(n.Capability),
			Sizing:     buildCanvasSizing(n.Sizing),
			RawAttributes: withCanvasNACLRules(
				withCanvasPlacement(
					withCanvasSecurityGroupRules(
						resolveCanvasCapabilityLevel(canvasCapabilityToRawAttributes(n.Capability), registry, n.ServiceID, n.Type),
						n.SecurityGroupRules,
					),
					n,
				),
				n.NACLRules,
			),
			Provenance: prov,
		})
	}

	edges := make([]core.Edge, 0, len(doc.Edges))
	for _, e := range doc.Edges {
		resolution := core.ResolutionKnown
		edgeProv := prov
		if !nodeExists[e.From] || !nodeExists[e.To] {
			resolution = core.ResolutionUnresolved
			edgeProv = prov.WithReason("references a node ID not present in this canvas document")
		}
		edges = append(edges, core.Edge{
			ID:         e.ID,
			Type:       e.Type,
			From:       e.From,
			To:         e.To,
			Resolution: resolution,
			Provenance: edgeProv,
		})
	}

	edges = append(edges, canvasRouteEdges(doc, nodeExists, prov)...)

	if insufficient := CheckMVG(nodes); insufficient != nil {
		return Result{Insufficient: insufficient}, nil
	}

	ir := &core.IR{
		SchemaVersion: "1.4.0",
		VersionNumber: versionNumber,
		VersionHash:   contentHash(nodes, edges),
		Nodes:         nodes,
		Edges:         edges,
	}

	return Result{IR: ir}, nil
}

// buildCanvasCapability reuses assignCapabilityField unchanged — see this file's own
// doc comment for why canvas capability keys are already canonical field names.
func buildCanvasCapability(capability map[string]string) *core.CapabilityModel {
	if len(capability) == 0 {
		return nil
	}
	cap := &core.CapabilityModel{}
	anySet := false
	for field, value := range capability {
		anySet = true
		assignCapabilityField(cap, field, value)
	}
	if !anySet {
		return nil
	}
	return cap
}

// buildCanvasSizing translates CanvasNode.Sizing's own canonical-field-named string
// map into a typed core.Sizing — PC-110's own UI-side counterpart to
// buildCanvasCapability above, same reasoning: canvas keys are already canonical
// core.Sizing field names (instance_type, count, ...), never a Terraform attribute
// name needing translation (that's ingest/sizing.go's own, separate job for
// Terraform-sourced resources). A key absent or blank leaves that Sizing field nil —
// cost_unknown for that dimension, never a guessed default (this ticket's own
// explicit acceptance criterion).
func buildCanvasSizing(sizing map[string]string) *core.Sizing {
	if len(sizing) == 0 {
		return nil
	}
	get := func(key string) *string {
		if v, ok := sizing[key]; ok && v != "" {
			return &v
		}
		return nil
	}
	getInt := func(key string) *int {
		v, ok := sizing[key]
		if !ok || v == "" {
			return nil
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil
		}
		return &n
	}
	s := core.Sizing{
		InstanceType:       get("instance_type"),
		TaskCPU:            get("task_cpu"),
		TaskMemory:         get("task_memory"),
		Count:              getInt("count"),
		InstanceClass:      get("instance_class"),
		AllocatedStorageGB: getInt("allocated_storage_gb"),
		StorageType:        get("storage_type"),
		CacheNodeType:      get("cache_node_type"),
		LoadBalancerType:   get("load_balancer_type"),
		Region:             get("region"),
	}
	if s == (core.Sizing{}) {
		return nil
	}
	return &s
}

// canvasCapabilityToRawAttributes retains the canvas's own declared capability
// key/values verbatim in RawAttributes too — mirroring core.Node.RawAttributes' own
// documented purpose ("raw provider attributes are additionally retained for
// attribute-level compliance checks", PRD §4), so a check reading a raw key directly
// (rather than through CapabilityModel) still finds a real, stated value entered via
// the canvas.
func canvasCapabilityToRawAttributes(capability map[string]string) map[string]any {
	if len(capability) == 0 {
		return nil
	}
	raw := make(map[string]any, len(capability))
	for k, v := range capability {
		raw[k] = v
	}
	return raw
}

// resolveCanvasCapabilityLevel is PC-136's own addition: resolves a canvas node's
// declared ServiceID into a PC-107 capability_level via the exact same
// providers.Registry.Lookup ingest/build.go's own withCapabilityLevel already calls
// for the Terraform path — no second lookup implementation, per this ticket's own
// explicit acceptance criterion. Three cases all leave capability_level unset,
// deliberately indistinguishable from each other at this layer (I4: an uncertain
// claim is not_assessable, never guessed, and this codebase does not rank different
// flavors of "uncertain"): ServiceID is empty (no service selected yet); ServiceID
// has no registry entry at all (unknown service, or a real service this registry
// simply hasn't mapped); or the registry entry's own NodeType disagrees with this
// node's declared Type (a real inconsistency — an ID borrowed from a different
// structural category — that this function refuses to trust rather than silently
// preferring one side). An unset capability_level is read by core/trace.go's own
// existing capability gate, which already reports not_assessable naming the node —
// the identical mechanism a real out-of-vocabulary Terraform resource already
// produces, not a new one built for canvas.
func resolveCanvasCapabilityLevel(raw map[string]any, registry providers.Registry, serviceID string, nodeType core.NodeType) map[string]any {
	if serviceID == "" {
		return raw
	}
	mapping, ok := registry.Lookup(serviceID)
	if !ok || mapping.IsEdgeMapping() || mapping.NodeType != nodeType {
		return raw
	}
	if raw == nil {
		raw = make(map[string]any, 1)
	}
	raw["capability_level"] = string(mapping.CapabilityLevel)
	return raw
}

// withCanvasNACLRules is PC-139's own addition: stamps a canvas node's authored NACL
// rules onto RawAttributes["nacl_rules"] in the EXACT shape ingest/nacl.go's
// normalizeNACLRule produces for Terraform (direction, number, protocol, from_port,
// to_port, cidr, allow bool) — core.NACLProfileForSubnet reads it back identically from
// either producer. No rule at all leaves the key unset: "not authored" is never an
// implied allow-all or deny-all (I4).
func withCanvasNACLRules(raw map[string]any, rules []core.CanvasNACLRule) map[string]any {
	if len(rules) == 0 {
		return raw
	}
	if raw == nil {
		raw = make(map[string]any, 1)
	}
	out := make([]map[string]any, 0, len(rules))
	for _, r := range rules {
		rule := map[string]any{
			"direction": r.Direction,
			"number":    r.Number,
			"protocol":  r.Protocol,
			"cidr":      r.CIDRBlock,
			"allow":     r.Action == "allow",
		}
		if r.FromPort != 0 {
			rule["from_port"] = r.FromPort
		}
		if r.ToPort != 0 {
			rule["to_port"] = r.ToPort
		}
		out = append(out, rule)
	}
	raw["nacl_rules"] = out
	return raw
}

// canvasRouteEdges is PC-138's own addition: every route authored on a route-table node
// becomes a routes_to edge from that node to the route's target, with destination_cidr
// and target_kind in RawAttributes — exactly the edge ingest/routes.go builds for a
// Terraform route, so core's routing engine reads one shape from both producers. A
// target the engine does not model, or that does not exist, yields an UNRESOLVED edge
// naming why (the server also rejects it up front) — never a silently dropped route.
func canvasRouteEdges(doc core.CanvasDocument, nodeExists map[string]bool, prov core.Provenance) []core.Edge {
	serviceOf := make(map[string]string, len(doc.Nodes))
	for _, n := range doc.Nodes {
		serviceOf[n.ID] = n.ServiceID
	}
	var out []core.Edge
	for _, n := range doc.Nodes {
		for i, r := range n.Routes {
			edge := core.Edge{
				ID:            fmt.Sprintf("%s-route[%d]->%s", n.ID, i, r.Target),
				Type:          core.EdgeTypeRoutesTo,
				From:          n.ID,
				To:            r.Target,
				Resolution:    core.ResolutionKnown,
				Provenance:    prov,
				RawAttributes: map[string]any{"destination_cidr": r.DestinationCIDR},
			}
			kind, supported := core.RouteTargetKind(serviceOf[r.Target])
			switch {
			case !nodeExists[r.Target]:
				edge.Resolution = core.ResolutionUnresolved
				edge.Provenance = prov.WithReason("route target " + r.Target + " is not a node in this canvas document")
			case !supported:
				edge.Resolution = core.ResolutionUnresolved
				edge.Provenance = prov.WithReason("route target " + r.Target + " is not an internet gateway or NAT gateway — that target kind is not modelled")
			default:
				edge.RawAttributes["target_kind"] = kind
			}
			out = append(out, edge)
		}
	}
	return out
}

// withCanvasPlacement is PC-105's own addition: stamps a canvas node's declared
// availability_zone / cidr_block onto RawAttributes under the SAME keys the Terraform
// path already uses for aws_subnet/aws_vpc, so every downstream reader sees one IR
// shape from both producers. Absent stays absent — never a defaulted zone or CIDR.
func withCanvasPlacement(raw map[string]any, n core.CanvasNode) map[string]any {
	if n.AvailabilityZone == "" && n.CIDRBlock == "" {
		return raw
	}
	if raw == nil {
		raw = make(map[string]any, 2)
	}
	if n.AvailabilityZone != "" {
		raw["availability_zone"] = n.AvailabilityZone
	}
	if n.CIDRBlock != "" {
		raw["cidr_block"] = n.CIDRBlock
	}
	return raw
}

// withCanvasSecurityGroupRules is PC-137's own addition: stamps a canvas node's
// authored SecurityGroupRules onto RawAttributes["security_group_rules"] in the
// EXACT rule-map shape ingest/securitygroups.go's own normalizeSGRule already
// produces for the Terraform path — core.SecurityGroupProfile reads this key back
// identically regardless of which producer built it, so there is exactly one SG
// evaluation path, not two. Absent or empty leaves the key unset entirely (never an
// empty slice standing in for "no rules were ever authored") — core/trace.go's own
// "zero attached SGs" check (this ticket's own companion fix) reads that absence as
// not_assessable, never an implied deny.
func withCanvasSecurityGroupRules(raw map[string]any, rules []core.CanvasSecurityGroupRule) map[string]any {
	if len(rules) == 0 {
		return raw
	}
	out := make([]map[string]any, 0, len(rules))
	for _, r := range rules {
		rule := map[string]any{"direction": r.Direction, "protocol": r.Protocol}
		if r.FromPort != 0 {
			rule["from_port"] = r.FromPort
		}
		if r.ToPort != 0 {
			rule["to_port"] = r.ToPort
		}
		if len(r.CIDRBlocks) > 0 {
			rule["cidr_blocks"] = r.CIDRBlocks
		}
		if r.SourceSecurityGroup != "" {
			rule["source_security_group"] = r.SourceSecurityGroup
		}
		out = append(out, rule)
	}
	if raw == nil {
		raw = make(map[string]any, 1)
	}
	raw["security_group_rules"] = out
	return raw
}
