import { applyNodeChanges, type Node, type NodeChange } from "@xyflow/react";
import type { CanvasNodeData } from "./types";
import { containerRank } from "./containment";

// React Flow resizers update width/height attributes. Keep the explicit container
// style in sync too: containment geometry uses that style while measurement catches
// up. A measured-size notification alone must never turn a service into a fixed box.
export function applyCanvasNodeChanges(changes: NodeChange<Node<CanvasNodeData>>[], nodes: Node<CanvasNodeData>[]): Node<CanvasNodeData>[] {
  const resized = new Set(changes.flatMap((c) => c.type === "dimensions" && c.setAttributes ? [c.id] : []));
  return applyNodeChanges(changes, nodes).map((n) => resized.has(n.id) && containerRank(n.data.serviceID) > 0
    ? { ...n, style: { ...n.style, width: n.width ?? n.style?.width, height: n.height ?? n.style?.height } }
    : n);
}
