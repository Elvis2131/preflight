import type { NodeType, EdgeType } from "./goldenVocabulary";

// CanvasDocument is the ONLY shape this app ever serializes to — PC-85's own
// acceptance criterion, verbatim: "canvas state (nodes + edges + capability values)
// is serialisable to a plain JSON document with no UI-only fields leaking into it."
// No position, no selection state, no React Flow internals (viewport, dragging,
// z-index, handle IDs) ever appear here — see serialize.ts, the one function
// permitted to produce this shape from React Flow's own live state.
//
// As of PC-86, this is the MIRROR of core.CanvasDocument (preflight/core/canvas.go)
// — the sixth frozen contract, contracts/canvas.schema.json — not the reverse. A
// future change here (PC-87 adding real capability structure, for instance) means
// bumping that contract's version first, per contracts/CHANGELOG.md's own convention,
// not just editing this file.
export interface CanvasNode {
  id: string;
  type: NodeType;
  label: string;
  // capability: free-form key/value pairs, deliberately untyped at the shell stage.
  // PC-87 (the NFR form) and a later capability-form ticket own giving these real,
  // per-node-type structure and validation against workload.schema.json's own
  // semantics; this ticket is shell only (its own Card: "no backend wiring").
  capability: Record<string, string>;
  // sizing: PC-110's own addition — free-form key/value pairs entered via the Design
  // inspector, same untyped-at-the-shell-stage shape as capability above. Keys are
  // core.Sizing's own canonical field names (see sizingFields.ts) — real structure
  // lives in ingest/canvas.go's buildCanvasSizing, not here. Omitted (not an empty
  // object) when the architect has not opened the inspector for this node at all,
  // matching capability's own "absence means nothing entered" convention.
  sizing?: Record<string, string>;
}

export interface CanvasEdge {
  id: string;
  type: EdgeType;
  from: string;
  to: string;
}

export interface CanvasDocument {
  nodes: CanvasNode[];
  edges: CanvasEdge[];
}

// CanvasNodeData is what actually lives in a React Flow Node's `data` field — the
// UI-side working state. Distinct from CanvasNode (the serialized shape) on purpose:
// this is where a future ticket could add UI-only concerns (e.g. a "just added,
// highlight me" flag) without that ever leaking into CanvasDocument, since
// serialize.ts only ever reads the three fields below out of it.
// SimState is UI-only presentation state from a PC-88 "kill this node" simulation
// result — never read by serialize.ts, never present in CanvasDocument. "killed" is
// the node the user actually targeted; "severed" mirrors /simulate's own
// severed_paths exactly (a stateful node whose path no longer survives); "cascaded"
// is any OTHER node in /simulate's cascade[] (e.g. an unreachable non-stateful node)
// — kept visually distinct from "severed" so the one true acceptance signal
// (severed_paths) is never diluted by a broader "affected" set.
export type SimState = "normal" | "killed" | "severed" | "cascaded";

export interface CanvasNodeData extends Record<string, unknown> {
  nodeType: NodeType;
  label: string;
  capability: Record<string, string>;
  sizing?: Record<string, string>;
  simState?: SimState;
}

export interface CanvasEdgeData extends Record<string, unknown> {
  edgeType: EdgeType;
  severed?: boolean;
}
