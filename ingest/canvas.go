// This file is PC-86's core: IngestCanvas is a second IR producer, symmetric to
// Ingest (HCL/Terraform) — both feed the exact same downstream pipeline (MVG check,
// core/analyse, core/simulate) and produce the identical IR shape. No canvas-specific
// field or shortcut exists anywhere in core/ — everything canvas-specific stays in
// this one file, on this side of the ingestion boundary, per this ticket's own
// acceptance criterion verbatim.
package ingest

import (
	"preflight/core"
)

// IngestCanvas builds an IR directly from a canvas-authored core.CanvasDocument
// (PC-85's own wire contract, frozen as contracts/canvas.schema.json). Unlike Ingest,
// there is no provider-mapping Registry involved: a canvas node already declares its
// canonical NodeType directly — the palette IS the golden vocabulary (PC-85) — so
// there is no raw resource_type string to map through a providers.Registry the way an
// HCL resource has.
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
func IngestCanvas(doc core.CanvasDocument, versionNumber int) (Result, error) {
	nodeExists := make(map[string]bool, len(doc.Nodes))
	for _, n := range doc.Nodes {
		nodeExists[n.ID] = true
	}

	prov := core.NewProvenance(core.KindStated, "canvas")

	nodes := make([]core.Node, 0, len(doc.Nodes))
	for _, n := range doc.Nodes {
		nodes = append(nodes, core.Node{
			ID:            n.ID,
			Type:          n.Type,
			Resolution:    core.ResolutionKnown,
			Capability:    buildCanvasCapability(n.Capability),
			RawAttributes: canvasCapabilityToRawAttributes(n.Capability),
			Provenance:    prov,
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

	if insufficient := CheckMVG(nodes); insufficient != nil {
		return Result{Insufficient: insufficient}, nil
	}

	ir := &core.IR{
		SchemaVersion: "1.3.0",
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
