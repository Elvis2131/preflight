import { describe, it, expect } from "vitest";
import type { Template } from "./api";
import { templateToCanvasState } from "./templateLoader";
import { serialize } from "./serialize";
import { reevaluate, AUTO_EDGE_PREFIX } from "./containment";
import { workloadToFormValue, buildWorkload } from "./WorkloadForm";

// A small template with one single-parent container edge (subnet -> vpc), one
// two-parent edge (lb -> two subnets) and one edge geometry cannot own (db -> a
// non-container group).
const t: Template = {
  meta: { id: "t", name: "T", description: "d" },
  workload: {
    schema_version: "1.0.0", name: "w", criticality: "tier1", data_classification: "internal",
    regions: ["eu-west-1"], requirements: [{ id: "rto_seconds", value: "60", priority: "hard" }],
  } as unknown as Template["workload"],
  layout: {
    vpc: { x: 0, y: 0, width: 700, height: 400 },
    sa: { x: 20, y: 40, width: 300, height: 190 },
    sb: { x: 340, y: 40, width: 300, height: 190 },
    lb: { x: 60, y: 90 },
    grp: { x: 60, y: 260 },
    db: { x: 60, y: 320 },
  },
  canvas: {
    nodes: [
      { id: "vpc", type: "network_boundary", label: "VPC", capability: {}, service_id: "aws_vpc", cidr_block: "10.0.0.0/16" },
      { id: "sa", type: "network_boundary", label: "A", capability: {}, service_id: "aws_subnet", availability_zone: "eu-west-1a" },
      { id: "sb", type: "network_boundary", label: "B", capability: {}, service_id: "aws_subnet", availability_zone: "eu-west-1b" },
      { id: "lb", type: "load_balancer", label: "LB", capability: {}, service_id: "aws_lb" },
      { id: "grp", type: "network_boundary", label: "Grp", capability: {}, service_id: "aws_db_subnet_group" },
      { id: "db", type: "managed_database", label: "DB", capability: { storage_encrypted: "true" }, service_id: "aws_db_instance" },
    ],
    edges: [
      { id: "e1", type: "contained_in", from: "sa", to: "vpc" },
      { id: "e2", type: "contained_in", from: "sb", to: "vpc" },
      { id: "e3", type: "contained_in", from: "lb", to: "sa" },
      { id: "e4", type: "contained_in", from: "lb", to: "sb" },
      { id: "e5", type: "contained_in", from: "db", to: "grp" },
      { id: "e6", type: "routes_to", from: "lb", to: "db" },
    ],
  },
};

describe("templateToCanvasState (PC-108)", () => {
  const { nodes, edges } = templateToCanvasState(t);

  it("round-trips: serialize() gives back exactly the template's canvas document", () => {
    const doc = serialize(nodes, edges);
    const norm = (d: typeof doc) => ({
      nodes: [...d.nodes].sort((a, b) => a.id.localeCompare(b.id)),
      edges: d.edges.map((e) => ({ type: e.type, from: e.from, to: e.to })).sort((a, b) => (a.from + a.to + a.type).localeCompare(b.from + b.to + b.type)),
    });
    expect(norm(doc)).toEqual(norm(t.canvas));
  });

  it("containers get explicit size and layering; layout never reaches the document", () => {
    const vpc = nodes.find((n) => n.id === "vpc")!;
    expect(vpc.style).toEqual({ width: 700, height: 400 });
    const out = serialize(nodes, edges).nodes[0] as unknown as Record<string, unknown>;
    for (const k of ["position", "style", "zIndex", "layout"]) expect(Object.keys(out)).not.toContain(k);
  });

  it("only an edge geometry owns gets the auto id; a two-parent node and a non-container parent keep theirs", () => {
    const ids = edges.map((e) => e.id);
    expect(ids).toContain(AUTO_EDGE_PREFIX + "sa");
    expect(ids).toContain(AUTO_EDGE_PREFIX + "sb");
    expect(ids).toContain("e3");
    expect(ids).toContain("e4");
    expect(ids).toContain("e5");
  });

  it("re-evaluating after load never adds a second parent to a hand-kept node, and keeps the auto ones", () => {
    const again = reevaluate(nodes, edges, nodes.map((n) => n.id));
    const parents = (es: typeof edges, id: string) =>
      es.filter((e) => e.source === id && e.data?.edgeType === "contained_in").map((e) => e.target).sort();
    expect(parents(again, "db")).toEqual(["grp"]); // placed via its group, not directly in a subnet or the VPC
    expect(parents(again, "lb")).toEqual(["sa", "sb"]); // spans two subnets, untouched
    expect(parents(again, "sa")).toEqual(["vpc"]);
    expect(parents(again, "sb")).toEqual(["vpc"]);
  });
});

describe("workloadToFormValue", () => {
  it("round-trips through buildWorkload without inventing capacity or journeys", () => {
    const form = workloadToFormValue(t.workload);
    const w = buildWorkload(form);
    expect(w.name).toBe("w");
    expect(w.regions).toEqual(["eu-west-1"]);
    expect(w.capacity).toBeUndefined();
    expect(w.journeys).toBeUndefined();
    expect(w.requirements?.[0]).toMatchObject({ id: "rto_seconds", priority: "hard" });
  });
});
