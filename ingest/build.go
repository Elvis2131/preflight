package ingest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"preflight/core"
	"preflight/providers"
)

// OutOfVocabularyResource is a parsed resource whose Terraform type has no provider
// mapping — i.e. it is outside the golden vocabulary (CLAUDE.md §14/§27: "a resource
// outside this vocabulary surfaces as not_assessable, never silently ignored"). It is
// tracked here, separately from core.IR.Nodes, rather than forced into the closed
// NodeType enum or silently dropped — this is what makes PC-12's third acceptance
// criterion ("round-trips without loss") actually true: every parsed resource ends up
// in exactly one of IR.Nodes or this list, never neither.
type OutOfVocabularyResource struct {
	ResourceType string
	ResourceName string
	SourceFile   string
	SourceLine   int
}

// EdgeOnlyResource records a resource that was mapped, but produces an edge rather
// than a node (providers/aws.EdgeMapping — e.g. aws_wafv2_web_acl_association). It is
// tracked separately from both IR.Nodes and OutOfVocabulary so the round-trip
// accounting (every parsed resource ends up in exactly one bucket) stays exhaustive:
// an edge-only resource is neither an out-of-vocabulary resource (it IS mapped) nor a
// node (it deliberately produces none).
type EdgeOnlyResource struct {
	ResourceType string
	ResourceName string
	Produced     bool // false if its from/to references could not both be resolved
}

// Result is ingest's top-level output: either a usable IR, or a structured
// insufficient-model response — never both, and never a set of findings computed
// against a graph too small to support them (PC-12's Card).
type Result struct {
	IR                *core.IR
	Insufficient      *InsufficientModel
	OutOfVocabulary   []OutOfVocabularyResource
	EdgeOnlyResources []EdgeOnlyResource

	// UnsupportedRoutes is PC-111's own acceptance criterion: every route this
	// codebase recognizes but doesn't yet model (VPC peering, transit gateway, VPC
	// endpoint, ...) — never silently dropped, never treated as a blackhole or as
	// success. The caller (server.Assess, cmd/gen-golden-fixtures) turns each one into
	// a real not_assessable Finding via core.BuildUnsupportedRouteFindings.
	UnsupportedRoutes []UnsupportedRoute
}

// Ingest parses dir, maps resources through registry, runs the MVG check, and returns
// exactly one of a usable IR or a structured InsufficientModel.
func Ingest(dir string, registry providers.Registry, versionNumber int) (Result, error) {
	parsed, err := ParseDir(dir)
	if err != nil {
		return Result{}, err
	}

	byKey := map[string]ParsedResource{}
	for _, r := range parsed {
		byKey[r.Key()] = r
	}

	var nodes []core.Node
	var oov []OutOfVocabularyResource
	var edgeOnly []EdgeOnlyResource
	var mappedEdges []core.Edge

	for _, r := range parsed {
		if r.Type == "aws_route" {
			// PC-111: aws_route is a real, recognized resource that produces neither
			// a node nor a generic mapped edge — its own route (destination CIDR ->
			// target) is built by buildRouteEdges below, which needs the full
			// ParsedResource (nested attributes + references), not the node/edge-only
			// shape this loop otherwise assumes. Tracked in edgeOnly (not oov: it IS
			// recognized) once buildRouteEdges below reports whether it produced a
			// real edge — see the EdgeOnlyResources append after buildRouteEdges runs.
			continue
		}
		if r.Type == "aws_security_group_rule" {
			// PC-112: same reasoning as aws_route above — a real, recognized resource
			// that produces no node/generic edge of its own; its rule data is merged
			// onto its owning aws_security_group node by mergeSecurityGroupRules,
			// which needs the full ParsedResource, not this loop's node/edge-only
			// shape. Tracked in edgeOnly, not oov.
			owner, hasOwner := r.AttributeReferences["security_group_id"]
			edgeOnly = append(edgeOnly, EdgeOnlyResource{ResourceType: r.Type, ResourceName: r.Name, Produced: hasOwner && len(owner) > 0})
			continue
		}
		mapping, ok := registry.Lookup(r.Type)
		if !ok {
			oov = append(oov, OutOfVocabularyResource{
				ResourceType: r.Type, ResourceName: r.Name,
				SourceFile: r.SourceFile, SourceLine: r.SourceLine,
			})
			continue
		}
		if mapping.IsEdgeMapping() {
			edge, ok := buildMappedEdge(r, mapping, byKey, registry)
			edgeOnly = append(edgeOnly, EdgeOnlyResource{ResourceType: r.Type, ResourceName: r.Name, Produced: ok})
			if ok {
				mappedEdges = append(mappedEdges, edge)
			}
			continue
		}
		nodes = append(nodes, buildNode(r, mapping))
	}

	if insufficient := CheckMVG(nodes); insufficient != nil {
		return Result{Insufficient: insufficient, OutOfVocabulary: oov, EdgeOnlyResources: edgeOnly}, nil
	}

	routeEdges, unsupportedRoutes, ownedRouteRefs, routeProducedFor := buildRouteEdges(parsed, byKey, registry)
	for _, r := range parsed {
		if r.Type != "aws_route" {
			continue
		}
		edgeOnly = append(edgeOnly, EdgeOnlyResource{ResourceType: r.Type, ResourceName: r.Name, Produced: routeProducedFor[r.Key()]})
	}

	edges := buildEdges(parsed, byKey, registry, ownedRouteRefs)
	edges = append(edges, mappedEdges...)
	edges = append(edges, routeEdges...)
	mergeSecurityGroupRules(nodes, parsed)

	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	sort.Slice(edges, func(i, j int) bool { return edges[i].ID < edges[j].ID })

	ir := &core.IR{
		SchemaVersion: "1.1.0",
		VersionNumber: versionNumber,
		VersionHash:   contentHash(nodes, edges),
		Nodes:         nodes,
		Edges:         edges,
	}
	return Result{IR: ir, OutOfVocabulary: oov, EdgeOnlyResources: edgeOnly, UnsupportedRoutes: unsupportedRoutes}, nil
}

// buildMappedEdge resolves an EdgeMapping's from/to attributes to their referenced
// nodes and produces the declared edge. Returns ok=false (not an error — this is a
// normal, trackable outcome, not a parse failure) if either attribute's reference
// could not be resolved to a node that actually exists in this bundle — e.g. the
// association points at a resource that was itself filtered out as out-of-vocabulary,
// or a reference is simply absent from this particular resource instance.
func buildMappedEdge(r ParsedResource, mapping providers.ResourceMapping, byKey map[string]ParsedResource, registry providers.Registry) (core.Edge, bool) {
	fromRef, ok := soleReference(r, mapping.Edge.FromAttribute)
	if !ok {
		return core.Edge{}, false
	}
	toRef, ok := soleReference(r, mapping.Edge.ToAttribute)
	if !ok {
		return core.Edge{}, false
	}

	// Both endpoints must themselves be mapped, real nodes — an edge pointing at a
	// resource this registry has no node for would be a dangling reference at the IR
	// level, which is not what happened here (the association resource itself parsed
	// fine); it is simply not representable as an edge between two IR nodes yet.
	if !nodeWillExist(fromRef, byKey, registry) || !nodeWillExist(toRef, byKey, registry) {
		return core.Edge{}, false
	}

	prov := core.NewProvenance(core.KindStated, sourceRef(r))
	return core.Edge{
		ID:         fromRef.Key() + "->" + toRef.Key() + " (via " + r.Key() + ")",
		Type:       mapping.Edge.Type,
		From:       fromRef.Key(),
		To:         toRef.Key(),
		Resolution: core.ResolutionKnown,
		Provenance: prov,
	}, true
}

func soleReference(r ParsedResource, attribute string) (ResourceRef, bool) {
	refs := r.AttributeReferences[attribute]
	if len(refs) != 1 {
		return ResourceRef{}, false
	}
	return refs[0], true
}

func nodeWillExist(ref ResourceRef, byKey map[string]ParsedResource, registry providers.Registry) bool {
	target, exists := byKey[ref.Key()]
	if !exists {
		return false
	}
	m, mapped := registry.Lookup(target.Type)
	return mapped && !m.IsEdgeMapping()
}

func buildNode(r ParsedResource, mapping providers.ResourceMapping) core.Node {
	resolution := core.ResolutionKnown
	prov := core.NewProvenance(core.KindStated, sourceRef(r))

	if r.HasCount || r.HasForEach || r.HasDynamicBlock {
		resolution = core.ResolutionUnresolved
		prov = prov.WithReason(unresolvedConstructReason(r))
	}

	return core.Node{
		ID:            r.Key(),
		Type:          mapping.NodeType,
		Resolution:    resolution,
		Capability:    buildCapability(r, mapping),
		RawAttributes: r.Attributes,
		Provenance:    prov,
	}
}

func unresolvedConstructReason(r ParsedResource) string {
	switch {
	case r.HasCount:
		return "resource uses count, outside the v1 parser subset (CLAUDE.md §15)"
	case r.HasForEach:
		return "resource uses for_each, outside the v1 parser subset (CLAUDE.md §15)"
	default:
		return "resource contains a dynamic block, outside the v1 parser subset (CLAUDE.md §15)"
	}
}

// buildCapability populates capability fields from the resource's parsed literal
// attributes: present in the source -> the value is used (Kind stated, at the node
// level — see the limitation note below). Absent -> the field is left nil, REGARDLESS
// of whether the mapping declares on_absent=assumed or on_absent=not_assessable.
//
// DELIBERATE SCOPE LIMIT, not an oversight: PC-13's mapping data DOES carry a
// doc-verified default for several assumed fields (e.g. ALB idle_timeout=60s), but this
// builder does not synthesize that default into the IR. Reason: core.Node carries one
// Provenance for the WHOLE node (a v1 simplification contracts/CHANGELOG.md already
// names as open), not one per capability field. Writing an assumed default into a node
// whose OTHER fields are genuinely stated would force that node's single Provenance.Kind
// to claim "stated" for a value that is actually "assumed" — exactly the I2 violation
// per-field provenance exists to prevent. Applying assumed defaults correctly requires
// the per-field provenance PC-11's fuller IR design is already tracked as owning; doing
// it here first would be solving the same problem twice, incompatibly. The mapping data
// itself remains a real, inspectable, doc-verified artifact in the meantime — see
// providers/aws's own tests.
func buildCapability(r ParsedResource, mapping providers.ResourceMapping) *core.CapabilityModel {
	if len(mapping.Capabilities) == 0 && mapping.Failover == nil {
		return nil
	}
	cap := &core.CapabilityModel{}
	anySet := false

	for _, c := range mapping.Capabilities {
		val, present := r.Attributes[c.SourceAttribute]
		if !present {
			continue // not_assessable and assumed are both represented the same way
			// at the CapabilityModel field level today: the field is nil either way.
			// The distinction PC-13 asks for (assumed carries a documented default)
			// lives in the mapping data itself (inspectable via the registry), not
			// duplicated onto every Node — see the note in the doc comment above.
		}
		anySet = true
		assignCapabilityField(cap, c.Field, val)
	}

	if mapping.Failover != nil {
		if rm, fm := deriveFailover(r, *mapping.Failover); rm != nil {
			cap.ReplicationMode = rm
			cap.FailoverMechanism = fm
			anySet = true
		}
	}

	if !anySet {
		return nil
	}
	return cap
}

// deriveFailover applies one resource type's FailoverMapping (PC-22, providers/aws/
// mapping.go) against its parsed attributes. Absence of the source attribute is always
// not_assessable (nil, nil) — no assumed-default path exists here, matching
// buildCapability's own established restraint for every other field (see its doc
// comment above). A present-but-FALSE value always yields core.FailoverMechanismNone
// for BOTH outputs — a structural fact true regardless of provider ("no failover
// mechanism was declared"), hardcoded here rather than left as freely-typed YAML data,
// specifically to avoid reintroducing the exact sentinel-drift bug PC-14 already found
// and fixed once (see that constant's own doc comment in core/internal/analyse).
func deriveFailover(r ParsedResource, m providers.FailoverMapping) (replicationMode, failoverMechanism *string) {
	val, present := r.Attributes[m.SourceAttribute]
	if !present {
		return nil, nil
	}
	enabled, ok := val.(bool)
	if !ok {
		return nil, nil
	}
	if !enabled {
		none := core.FailoverMechanismNone
		return &none, &none
	}
	rm, fm := m.WhenEnabled.ReplicationMode, m.WhenEnabled.FailoverMechanism
	return &rm, &fm
}

// assignCapabilityField writes a mapped literal value into CapabilityModel's matching
// field by name. This is the one place resource-shape knowledge and CapabilityModel's
// shape meet — necessarily some code, since Go has no data-driven struct-field-by-
// string-name assignment without reflection, which would be a worse trade for a fixed,
// small field set than a plain switch.
func assignCapabilityField(cap *core.CapabilityModel, field string, val any) {
	s, ok := val.(string)
	if !ok {
		s = fmt.Sprintf("%v", val)
	}
	switch field {
	case "replication_mode":
		cap.ReplicationMode = &s
	case "failover_mechanism":
		cap.FailoverMechanism = &s
	case "failover_time_class":
		cap.FailoverTimeClass = &s
	case "backup_semantics", "backup_retention_period_days":
		cap.BackupSemantics = &s
	case "encryption_mechanism", "server_side_encryption", "storage_encrypted",
		"secrets_envelope_encryption", "transit_encryption_enabled", "at_rest_encryption_enabled":
		cap.EncryptionMechanism = &s
	case "key_ownership":
		cap.KeyOwnership = &s
	case "multi_az_implementation", "multi_az", "automatic_failover_enabled":
		cap.MultiAZImplementation = &s
	case "maintenance_behaviour":
		cap.MaintenanceBehaviour = &s
	// Fields with no direct CapabilityModel slot (deletion_protection, internal,
	// idle_timeout_seconds, control_plane_public_access, control_plane_private_access,
	// visibility_timeout_seconds, evaluate_target_health, routing_policy, scope,
	// default_action, trust_policy_present) are intentionally not mapped onto
	// CapabilityModel's fixed field set — PRD §4's own capability list (replication
	// mode, failover, backup, encryption, key ownership, multi-AZ, maintenance) does not
	// name a slot for them. They remain fully captured in RawAttributes instead ("raw
	// provider attributes are additionally retained for attribute-level compliance
	// checks" — PRD §4), not lost.
	default:
	}
}

// buildEdges walks references FROM each mapped, node-producing resource. Association-
// style resources (aws_wafv2_web_acl_association and any future equivalent) are
// handled separately by buildMappedEdge via providers/aws.EdgeMapping, since they
// produce no node of their own to walk references from — see the Ingest loop above.
//
// Remaining known limit of the EdgeMapping mechanism itself: soleReference requires
// exactly one reference per declared attribute, so an association attribute holding a
// LIST of targets (rather than a single reference) would not currently produce any
// edge at all — silently, from this function's point of view, though EdgeOnlyResource.
// Produced==false does make it visible in Result. Not a case golden/aws exercises
// today; worth a test the day a resource type that needs it is added.
func buildEdges(parsed []ParsedResource, byKey map[string]ParsedResource, registry providers.Registry, ownedRouteRefs map[string]bool) []core.Edge {
	var edges []core.Edge
	for _, r := range parsed {
		mapping, mapped := registry.Lookup(r.Type)
		if !mapped {
			continue // an edge FROM an out-of-vocabulary resource has no node to attach to
		}
		if mapping.IsEdgeMapping() {
			continue // this resource produces no node of its own — see buildMappedEdge,
			// which handles its references separately. Without this check, an
			// edge-only resource's references were being walked as if it were a real
			// node source, producing edges FROM a node ID that was never added to
			// IR.Nodes at all — caught by hand-verifying a generated fixture exactly
			// as PC-15's Conversation requires, not by a test that assumed the output
			// was already correct.
		}
		for _, ref := range r.References {
			// PC-111: a route table's own gateway_id/nat_gateway_id reference is
			// already accounted for as a real routes_to edge (with destination CIDR
			// data this generic walk cannot express) by buildRouteEdges — skip it here
			// so the same relationship doesn't ALSO appear as a second, data-less
			// depends_on edge.
			if ownedRouteRefs[r.Key()+"\x00"+ref.Key()] {
				continue
			}
			target, exists := byKey[ref.Key()]
			resolution := core.ResolutionKnown
			reason := ""
			if !exists {
				resolution = core.ResolutionUnresolved
				reason = "references " + ref.Key() + ", a declaration not present in the bundle (PRD §4)"
			} else if _, targetMapped := registry.Lookup(target.Type); !targetMapped {
				// Target exists in the bundle but is outside the golden vocabulary —
				// a real, present resource, just not one with a canonical node yet.
				// Not a dangling reference (PRD §4's specific unresolved definition),
				// so this is its own edge case: skip the edge rather than pointing it
				// at a node that doesn't exist, tracked instead via OutOfVocabulary.
				continue
			}

			// PC-78: the edge type is the TARGET node type's own declared
			// ReferenceEdgeType (e.g. a subnet says references to it are
			// containment), not always depends_on. Defaults to depends_on when the
			// target mapping doesn't set one — the long-standing behavior for the
			// original 8 golden types, unchanged.
			edgeType := core.EdgeTypeDependsOn
			if exists {
				if targetMapping, ok := registry.Lookup(target.Type); ok && targetMapping.ReferenceEdgeType != "" {
					edgeType = targetMapping.ReferenceEdgeType
				}
			}

			prov := core.NewProvenance(core.KindStated, sourceRef(r))
			if reason != "" {
				prov = prov.WithReason(reason)
			}
			edges = append(edges, core.Edge{
				ID:         r.Key() + "->" + ref.Key(),
				Type:       edgeType,
				From:       r.Key(),
				To:         ref.Key(),
				Resolution: resolution,
				Provenance: prov,
			})
		}
	}
	return edges
}

func sourceRef(r ParsedResource) string {
	return fmt.Sprintf("%s:%d:%s", r.SourceFile, r.SourceLine, r.Key())
}

// contentHash is Design §5's "version hash" — a deterministic content hash so two IR
// documents can be compared for identity independent of VersionNumber bookkeeping.
// Nodes/edges are already sorted by ID before this is called, so the hash does not
// depend on parse order (CLAUDE.md §23: determinism, always).
func contentHash(nodes []core.Node, edges []core.Edge) string {
	buf, _ := json.Marshal(struct {
		Nodes []core.Node `json:"nodes"`
		Edges []core.Edge `json:"edges"`
	}{nodes, edges})
	sum := sha256.Sum256(buf)
	return "sha256:" + hex.EncodeToString(sum[:])
}
