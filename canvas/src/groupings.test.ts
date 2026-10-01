import { describe, it, expect } from "vitest";
import type { Node } from "@xyflow/react";
import type { CanvasNodeData } from "./types";
import type { SubnetFact } from "./api";
import { buildGroupings, GROUPING_ID_PREFIX } from "./groupings";
import { serialize } from "./serialize";

function subnet(id: string, x: number, y: number): Node<CanvasNodeData> {
  return { id, type: "golden", position: { x, y }, style: { width: 300, height: 190 }, data: { nodeType: "network_boundary", label: id, capability: {}, serviceID: "aws_subnet" } };
}
const fact = (node_id: string, az?: string, region?: string): SubnetFact => ({ node_id, availability_zone: az, region, visibility: "private" });

const nodes = [subnet("a1", 0, 0), subnet("a2", 0, 300), subnet("b1", 400, 0)];

describe("buildGroupings (PC-105: read-only Region/AZ groupings derived from server facts)", () => {
  const groups = buildGroupings(nodes, [fact("a1", "eu-west-1a", "eu-west-1"), fact("a2", "eu-west-1a", "eu-west-1"), fact("b1", "eu-west-1b", "eu-west-1")]);
  const byId = Object.fromEntries(groups.map((g) => [g.id, g]));

  it("draws one box per AZ and one per region, around exactly the subnets the facts name", () => {
    expect(Object.keys(byId).sort()).toEqual(["grouping:az:eu-west-1a", "grouping:az:eu-west-1b", "grouping:region:eu-west-1"]);
    const a = byId["grouping:az:eu-west-1a"];
    expect(a.data.members).toBe(2);
    // AZ a spans a1 (0..190) and a2 (300..490) vertically, plus padding
    expect(a.position.y).toBeLessThan(0);
    expect((a.style as { height: number }).height).toBeGreaterThan(490);
  });

  it("the region box encloses every AZ box (never coincides with them)", () => {
    const r = byId["grouping:region:eu-west-1"];
    for (const id of ["grouping:az:eu-west-1a", "grouping:az:eu-west-1b"]) {
      const g = byId[id];
      expect(r.position.x).toBeLessThan(g.position.x);
      expect(r.position.y).toBeLessThan(g.position.y);
      expect(r.position.x + (r.style as { width: number }).width).toBeGreaterThan(g.position.x + (g.style as { width: number }).width);
    }
  });

  it("carries explicit dimensions, so React Flow never leaves it visibility:hidden (found by the live run)", () => {
    for (const g of groups) {
      expect(g.width).toBe((g.style as { width: number }).width);
      expect(g.height).toBe((g.style as { height: number }).height);
      expect(g.width).toBeGreaterThan(0);
    }
  });

  it("is read-only in every way React Flow can express, and never an editable node", () => {
    for (const g of groups) {
      expect(g.draggable).toBe(false);
      expect(g.selectable).toBe(false);
      expect(g.connectable).toBe(false);
      expect(g.deletable).toBe(false);
      expect((g.style as { pointerEvents: string }).pointerEvents).toBe("none");
      expect(g.id.startsWith(GROUPING_ID_PREFIX)).toBe(true);
    }
  });

  it("never leaks into the CanvasDocument: serialize() reads only the real nodes", () => {
    expect(serialize(nodes, []).nodes.map((n) => n.id)).toEqual(["a1", "a2", "b1"]);
  });
});

describe("buildGroupings: no guessing", () => {
  it("a subnet with no AZ belongs to no box; one with an AZ but no derivable region gets an AZ box and no region box", () => {
    const g = buildGroupings(nodes, [fact("a1"), fact("a2", "us-west-2-lax-1"), fact("b1", "eu-west-1b", "eu-west-1")]);
    expect(g.map((x) => x.id).sort()).toEqual(["grouping:az:eu-west-1b", "grouping:az:us-west-2-lax-1", "grouping:region:eu-west-1"]);
    expect(g.find((x) => x.id === "grouping:az:us-west-2-lax-1")?.data.members).toBe(1);
  });

  it("a fact naming a node that is not on the canvas draws nothing", () => {
    expect(buildGroupings(nodes, [fact("ghost", "eu-west-1a", "eu-west-1")])).toEqual([]);
  });

  it("no facts, no groupings; output order is deterministic", () => {
    expect(buildGroupings(nodes, [])).toEqual([]);
    const facts = [fact("b1", "eu-west-1b", "eu-west-1"), fact("a1", "eu-west-1a", "eu-west-1")];
    expect(buildGroupings(nodes, facts).map((g) => g.id)).toEqual(buildGroupings(nodes, [...facts].reverse()).map((g) => g.id));
  });
});
