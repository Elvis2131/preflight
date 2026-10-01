import type { Node, Edge } from "@xyflow/react";
import type { CanvasNodeData, CanvasEdgeData } from "./types";
import type { Template } from "./api";
import { AUTO_EDGE_PREFIX, CONTAINER_SIZE, containerRank, innermostContainer } from "./containment";
import { NODE_TYPE_LABELS } from "./goldenVocabulary";

// templateToCanvasState (PC-108) turns a template into the SAME React Flow node/edge
// state a hand-drawn design has — nothing about the result says "template". Positions
// come from the template's UI-only layout; everything else is the canvas document.
//
// A contained_in edge whose parent is a drawn container (VPC/subnet) and whose node
// really sits inside it gets the auto-containment id, so dragging that node later
// REPLACES the edge instead of leaving a stale second parent behind (PC-105). Edges
// that geometry cannot own — a node spanning two subnets, a resource inside a
// non-container such as a DB subnet group — keep their own ids and stay hand-maintained.
export function templateToCanvasState(t: Template): { nodes: Node<CanvasNodeData>[]; edges: Edge<CanvasEdgeData>[] } {
  const nodes: Node<CanvasNodeData>[] = t.canvas.nodes.map((n) => {
    const pos = t.layout[n.id] ?? { x: 0, y: 0 };
    const rank = containerRank(n.service_id);
    const size = pos.width && pos.height ? { width: pos.width, height: pos.height } : CONTAINER_SIZE[n.service_id ?? ""];
    return {
      id: n.id,
      type: "golden",
      position: { x: pos.x, y: pos.y },
      ...(rank > 0 && size ? { style: { width: size.width, height: size.height }, zIndex: -(4 - rank) } : {}),
      data: {
        nodeType: n.type,
        label: n.label || NODE_TYPE_LABELS[n.type],
        capability: n.capability ?? {},
        ...(n.sizing ? { sizing: n.sizing } : {}),
        ...(n.service_id ? { serviceID: n.service_id } : {}),
        ...(n.security_group_rules ? { securityGroupRules: n.security_group_rules } : {}),
        ...(n.availability_zone ? { availabilityZone: n.availability_zone } : {}),
        ...(n.cidr_block ? { cidrBlock: n.cidr_block } : {}),
        ...(n.routes ? { routes: n.routes } : {}),
        ...(n.nacl_rules ? { naclRules: n.nacl_rules } : {}),
      },
    };
  });

  const parentCount = new Map<string, number>();
  for (const e of t.canvas.edges) {
    if (e.type === "contained_in") parentCount.set(e.from, (parentCount.get(e.from) ?? 0) + 1);
  }

  const edges: Edge<CanvasEdgeData>[] = t.canvas.edges.map((e) => {
    let id = e.id;
    if (e.type === "contained_in" && parentCount.get(e.from) === 1) {
      const child = nodes.find((n) => n.id === e.from);
      if (child && innermostContainer(child, nodes) === e.to) id = AUTO_EDGE_PREFIX + e.from;
    }
    return { id, source: e.from, target: e.to, type: "default", label: e.type, data: { edgeType: e.type } };
  });

  return { nodes, edges };
}
