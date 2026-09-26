import type { Node, Edge } from "@xyflow/react";
import type { CanvasNodeData, CanvasEdgeData, CanvasDocument } from "./types";

// serialize is the ONE function permitted to turn React Flow's live state into a
// CanvasDocument — PC-85's own acceptance criterion: "no UI-only fields leaking into
// it." React Flow's own Node/Edge carry position, selected, dragging, sourceHandle,
// draggable, and more (verified directly against @xyflow/system's real NodeBase/
// EdgeBase type declarations, not assumed) — every one of those is deliberately left
// out below. Only id/type/label/capability/sizing/service_id (nodes) and
// id/type/from/to (edges) ever cross into the serialized document.
export function serialize(
  nodes: Node<CanvasNodeData>[],
  edges: Edge<CanvasEdgeData>[],
): CanvasDocument {
  return {
    nodes: nodes.map((n) => ({
      id: n.id,
      type: n.data.nodeType,
      label: n.data.label,
      capability: n.data.capability,
      // sizing (PC-110) is only included when the architect actually entered at
      // least one value — an empty object would still be honest (buildCanvasSizing
      // treats both the same), but omitting it keeps a node nobody ever sized
      // looking identical to how it looked before this ticket.
      ...(n.data.sizing && Object.keys(n.data.sizing).length > 0 ? { sizing: n.data.sizing } : {}),
      // service_id (PC-136) is only included once the architect has actually picked
      // one — omitted, never an empty string, so "no service chosen yet" stays
      // indistinguishable-by-design from "chosen but blank" (there is no such state).
      ...(n.data.serviceID ? { service_id: n.data.serviceID } : {}),
    })),
    edges: edges.map((e) => ({
      id: e.id,
      type: (e.data?.edgeType ?? "depends_on") as CanvasEdgeData["edgeType"],
      from: e.source,
      to: e.target,
    })),
  };
}
