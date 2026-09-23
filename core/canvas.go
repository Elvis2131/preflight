package core

// CanvasDocument is the wire contract a canvas UI posts to become an assessable
// architecture (PC-86) — the SIXTH frozen contract, given the same treatment as the
// five PC-7 already froze (IR, provenance, workload, finding, ADR): generated into
// contracts/canvas.schema.json from this exact struct, versioned independently per
// Design §5.
//
// This is a deliberate decision, not deferred to "whatever serialize.ts happens to
// emit": canvas/src/types.ts's own CanvasDocument is now the MIRROR of this struct
// (the same relationship core/ir.go's NodeType/EdgeType already has with
// canvas/src/goldenVocabulary.ts — one source of truth, mirrored deliberately, not
// redefined) rather than the reverse. Without this, a future ticket (PC-87 adding
// capability structure, for instance) could change what the canvas emits with
// nothing on this side noticing until a real request failed oddly — exactly the
// class of silent producer/consumer drift this codebase has already found and fixed
// twice (the FailoverMechanismNone sentinel mismatch, PC-14; the BuildScorecard/
// ComputeDelta not_assessable exclusion, PC-83). A frozen, tested contract is the
// cheaper fix, paid once, rather than a repeat of that same class of bug at a third
// boundary.
type CanvasNode struct {
	ID    string   `json:"id" validate:"required" jsonschema:"required,minLength=1"`
	Type  NodeType `json:"type" validate:"required,oneof=compute container_workload managed_database cache load_balancer queue/stream object_store dns network_boundary identity external_dependency" jsonschema:"required"`
	Label string   `json:"label" validate:"required" jsonschema:"required,minLength=1"`

	// Capability: free-form string key/value pairs entered via the canvas UI.
	// Deliberately untyped at this contract's current version — PC-87 (the NFR form)
	// and a later capability-editing ticket own giving these real, per-node-type
	// structure and validation against workload.schema.json's own semantics. A
	// future version of THIS contract (canvas.schema.json bumped, per
	// contracts/CHANGELOG.md's own convention) is where that structure would land,
	// not a silent, undocumented shape change.
	Capability map[string]string `json:"capability" jsonschema:"description=Free-form capability key/value pairs entered via the canvas UI. Untyped in this contract version — see CanvasNode's own doc comment."`
}

type CanvasEdge struct {
	ID   string   `json:"id" validate:"required" jsonschema:"required,minLength=1"`
	Type EdgeType `json:"type" validate:"required,oneof=depends_on routes_to reads/writes authenticates_via replicates_to contained_in" jsonschema:"required"`
	From string   `json:"from" validate:"required" jsonschema:"required,minLength=1"`
	To   string   `json:"to" validate:"required" jsonschema:"required,minLength=1"`
}

type CanvasDocument struct {
	Nodes []CanvasNode `json:"nodes" validate:"dive" jsonschema:"required"`
	Edges []CanvasEdge `json:"edges" validate:"dive" jsonschema:"required"`
}

// Validate checks this CanvasDocument against the same struct tags
// contracts/canvas.schema.json is generated from.
func (d CanvasDocument) Validate() error {
	return validate.Struct(d)
}
