import type { Node, Edge } from "@xyflow/react";
import type { CanvasNodeData, CanvasEdgeData } from "./types";

// containment.ts (PC-105): UI-only geometry for "draw a resource inside a subnet and it
// IS contained_in that subnet". It decides nothing about validity — whether a placement
// is allowed is the SERVER's call (core.ValidateCanvasPlacement, error_code
// invalid_placement); this only turns where the architect dropped a node into the
// contained_in edge the document will carry, so the picture and the IR cannot disagree.
//
// Nesting model (mirrors core/canvas.go's CanvasNode doc): Region ⊃ VPC ⊃ AZ ⊃ Subnet ⊃
// resource is expressed as resource → subnet → VPC contained_in edges plus the subnet's
// own availability_zone attribute. An AZ is an attribute, not a node, as in Terraform.

const RANK: Record<string, number> = { aws_vpc: 1, aws_subnet: 2 };
const LEAF_RANK = 3;

// Default drawn size of a container, applied when a node becomes a VPC/subnet.
export const CONTAINER_SIZE: Record<string, { width: number; height: number }> = {
  aws_vpc: { width: 560, height: 380 },
  aws_subnet: { width: 260, height: 190 },
};

// An auto-created edge is recognisable by id so it can be replaced when the node moves
// without ever touching a contained_in edge the architect drew by hand (e.g. the second
// subnet an Application Load Balancer must span).
export const AUTO_EDGE_PREFIX = "auto-contain-";

export function containerRank(serviceID: string | undefined): number {
  return serviceID ? (RANK[serviceID] ?? 0) : 0;
}

function rankOf(n: Node<CanvasNodeData>): number {
  return containerRank(n.data.serviceID) || LEAF_RANK;
}

function size(n: Node<CanvasNodeData>): { w: number; h: number } {
  const style = (n.style ?? {}) as { width?: number; height?: number };
  // An explicit style size (a drawn container) wins over React Flow's measured size:
  // measurement is asynchronous, so right after a node becomes a container `measured`
  // still holds its old, small size and the container would look 150px wide.
  return {
    w: style.width ?? n.measured?.width ?? n.width ?? 150,
    h: style.height ?? n.measured?.height ?? n.height ?? 60,
  };
}

// innermostContainer returns the id of the deepest container whose rectangle holds the
// centre of `node`. Auto-placement is deliberately conservative: a subnet may be
// auto-contained by a VPC, and ordinary resources may be auto-contained by a subnet.
// A resource dropped straight onto a VPC is left unlinked so the user can decide whether
// to draw a relationship.
export function innermostContainer(node: Node<CanvasNodeData>, all: Node<CanvasNodeData>[]): string | null {
  const { w, h } = size(node);
  const cx = node.position.x + w / 2;
  const cy = node.position.y + h / 2;
  const myRank = rankOf(node);
  let best: { id: string; rank: number; area: number } | null = null;
  for (const c of all) {
    const rank = containerRank(c.data.serviceID);
    if (c.id === node.id || rank === 0 || rank >= myRank) continue;
    if (myRank === LEAF_RANK && rank !== RANK.aws_subnet) continue;
    const cs = size(c);
    const inside =
      cx > c.position.x && cx < c.position.x + cs.w && cy > c.position.y && cy < c.position.y + cs.h;
    if (!inside) continue;
    const area = cs.w * cs.h;
    if (!best || rank > best.rank || (rank === best.rank && area < best.area)) {
      best = { id: c.id, rank, area };
    }
  }
  return best ? best.id : null;
}

// withAutoContainment replaces nodeId's auto-created contained_in edge so it points at
// containerId (or removes it when null). Hand-drawn edges are never touched.
export function withAutoContainment(
  edges: Edge<CanvasEdgeData>[],
  nodeId: string,
  containerId: string | null,
): Edge<CanvasEdgeData>[] {
  const autoId = AUTO_EDGE_PREFIX + nodeId;
  const kept = edges.filter((e) => e.id !== autoId);
  if (!containerId) return kept;
  // A hand-kept contained_in edge to this very container already says it (a template's
  // multi-subnet node, say) — adding the auto one would duplicate it.
  if (kept.some((e) => e.source === nodeId && e.target === containerId && e.data?.edgeType === "contained_in")) {
    return kept;
  }
  return [
    ...kept,
    {
      id: autoId,
      source: nodeId,
      target: containerId,
      type: "default",
      label: "contained_in",
      data: { edgeType: "contained_in" },
    },
  ];
}

// reevaluate recomputes the auto containment edge of every node in `ids` from where the
// nodes currently sit.
export function reevaluate(
  nodes: Node<CanvasNodeData>[],
  edges: Edge<CanvasEdgeData>[],
  ids: string[],
): Edge<CanvasEdgeData>[] {
  let out = edges;
  for (const id of ids) {
    const n = nodes.find((x) => x.id === id);
    if (!n) continue;
    // A node whose containment is kept by hand (a database placed via its subnet
    // group, a load balancer spanning two subnets) is the architect's to maintain:
    // geometry must not bolt a second, direct parent onto it.
    if (out.some((e) => e.source === id && e.data?.edgeType === "contained_in" && !e.id.startsWith(AUTO_EDGE_PREFIX))) {
      continue;
    }
    out = withAutoContainment(out, id, innermostContainer(n, nodes));
  }
  return out;
}
