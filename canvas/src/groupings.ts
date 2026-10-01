import type { Node } from "@xyflow/react";
import type { CanvasNodeData } from "./types";
import type { SubnetFact } from "./api";

// groupings.ts (PC-105, amended by its decision comment): Region and AZ are drawn as
// READ-ONLY visual groupings derived from subnet attributes, never authored on the picture.
// Which subnet is in which AZ/region is the SERVER's derivation (core.DeriveSubnetFacts, from
// the subnet's availability_zone; region from the documented AZ-code shape); this file only
// turns those facts into rectangles around the subnets they name. Nothing here is serialized
// into the CanvasDocument — the grouping nodes exist only in what React Flow is handed to
// draw — and nothing on a grouping can be edited, so the canvas has no way to contradict the
// model: move the subnet's AZ attribute and the box follows.

export type GroupingKind = "az" | "region";

export interface GroupingData extends Record<string, unknown> {
  kind: GroupingKind;
  label: string;
  members: number;
}

export const GROUPING_NODE_TYPE = "grouping";
export const GROUPING_ID_PREFIX = "grouping:";

// Padding around the members; a region wraps its AZ boxes with a larger margin so the two
// outlines never coincide.
const AZ_PAD = 28;
const REGION_PAD = 56;

interface Box {
  x: number;
  y: number;
  w: number;
  h: number;
}

function boxOf(n: Node<CanvasNodeData>): Box {
  const style = (n.style ?? {}) as { width?: number; height?: number };
  return {
    x: n.position.x,
    y: n.position.y,
    w: style.width ?? n.measured?.width ?? n.width ?? 150,
    h: style.height ?? n.measured?.height ?? n.height ?? 60,
  };
}

function union(boxes: Box[], pad: number): Box {
  const x1 = Math.min(...boxes.map((b) => b.x)) - pad;
  const y1 = Math.min(...boxes.map((b) => b.y)) - pad;
  const x2 = Math.max(...boxes.map((b) => b.x + b.w)) + pad;
  const y2 = Math.max(...boxes.map((b) => b.y + b.h)) + pad;
  return { x: x1, y: y1, w: x2 - x1, h: y2 - y1 };
}

function groupingNode(id: string, kind: GroupingKind, label: string, members: number, b: Box, z: number): Node<GroupingData> {
  return {
    id: GROUPING_ID_PREFIX + id,
    type: GROUPING_NODE_TYPE,
    position: { x: b.x, y: b.y },
    // Explicit width/height: React Flow keeps a node `visibility: hidden` until it has
    // dimensions, and a display-only box nobody measures would otherwise never appear.
    width: b.w,
    height: b.h,
    style: { width: b.w, height: b.h, pointerEvents: "none" },
    zIndex: z,
    draggable: false,
    selectable: false,
    connectable: false,
    focusable: false,
    deletable: false,
    data: { kind, label, members },
  };
}

// buildGroupings returns the AZ and region boxes for the given server-derived subnet facts.
// A subnet whose AZ is unset belongs to no AZ box; one whose region could not be derived
// (a Local/Wavelength Zone code) belongs to no region box — never guessed into one. Sorted
// by id so the drawn order never depends on iteration order.
export function buildGroupings(nodes: Node<CanvasNodeData>[], facts: SubnetFact[]): Node<GroupingData>[] {
  const byID = new Map(nodes.map((n) => [n.id, n]));
  const azBoxes = new Map<string, { box: Box; members: number; region: string }>();

  const byAZ = new Map<string, SubnetFact[]>();
  for (const f of facts) {
    if (!f.availability_zone || !byID.has(f.node_id)) continue;
    byAZ.set(f.availability_zone, [...(byAZ.get(f.availability_zone) ?? []), f]);
  }
  for (const [az, fs] of byAZ) {
    const boxes = fs.map((f) => boxOf(byID.get(f.node_id)!));
    azBoxes.set(az, { box: union(boxes, AZ_PAD), members: fs.length, region: fs.find((f) => f.region)?.region ?? "" });
  }

  const out: Node<GroupingData>[] = [];
  const regions = new Map<string, Array<{ box: Box; members: number }>>();
  for (const [az, v] of azBoxes) {
    out.push(groupingNode(`az:${az}`, "az", az, v.members, v.box, -5));
    if (v.region) regions.set(v.region, [...(regions.get(v.region) ?? []), { box: v.box, members: v.members }]);
  }
  for (const [region, vs] of regions) {
    out.push(groupingNode(`region:${region}`, "region", region, vs.reduce((n, v) => n + v.members, 0), union(vs.map((v) => v.box), REGION_PAD - AZ_PAD), -6));
  }
  return out.sort((a, b) => a.id.localeCompare(b.id));
}
