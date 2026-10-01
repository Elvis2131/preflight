import { describe, it, expect } from "vitest";
import type { Node, Edge } from "@xyflow/react";
import { serialize } from "./serialize";
import type { CanvasNodeData, CanvasEdgeData } from "./types";

// PC-85's own acceptance criterion, verbatim: "canvas state ... is serialisable to a
// plain JSON document with no UI-only fields leaking into it." This test constructs
// nodes/edges carrying the exact UI-only fields React Flow's own NodeBase/EdgeBase
// types define (position, selected, dragging, sourceHandle, etc. — verified directly
// against @xyflow/system's real type declarations, not guessed) and proves none of
// them survive serialization.

describe("serialize", () => {
  it("strips all UI-only fields from nodes", () => {
    const nodes: Node<CanvasNodeData>[] = [
      {
        id: "n1",
        type: "golden",
        position: { x: 123, y: 456 },
        selected: true,
        dragging: true,
        draggable: true,
        width: 200,
        height: 80,
        data: {
          nodeType: "managed_database",
          label: "Payments DB",
          capability: { encryption_mechanism: "true" },
        },
      },
    ];

    const doc = serialize(nodes, []);

    expect(doc.nodes).toEqual([
      {
        id: "n1",
        type: "managed_database",
        label: "Payments DB",
        capability: { encryption_mechanism: "true" },
      },
    ]);
    // Explicit negative check, not just a shape match: prove the UI-only keys are
    // genuinely absent, not merely unchecked.
    const serializedNode = doc.nodes[0] as unknown as Record<string, unknown>;
    for (const uiOnlyField of ["position", "selected", "dragging", "draggable", "width", "height"]) {
      expect(serializedNode[uiOnlyField]).toBeUndefined();
    }
  });

  it("includes sizing when the architect entered at least one value (PC-110)", () => {
    const nodes: Node<CanvasNodeData>[] = [
      {
        id: "n1",
        type: "golden",
        position: { x: 0, y: 0 },
        data: {
          nodeType: "managed_database",
          label: "DB",
          capability: {},
          sizing: { instance_class: "db.r6g.xlarge" },
        },
      },
    ];
    const doc = serialize(nodes, []);
    expect(doc.nodes[0]).toEqual({
      id: "n1",
      type: "managed_database",
      label: "DB",
      capability: {},
      sizing: { instance_class: "db.r6g.xlarge" },
    });
  });

  it("omits sizing entirely when nothing was entered — never an empty object", () => {
    const nodes: Node<CanvasNodeData>[] = [
      { id: "n1", type: "golden", position: { x: 0, y: 0 }, data: { nodeType: "compute", label: "App", capability: {} } },
    ];
    const doc = serialize(nodes, []);
    expect(Object.prototype.hasOwnProperty.call(doc.nodes[0], "sizing")).toBe(false);
  });

  it("omits sizing when present but empty", () => {
    const nodes: Node<CanvasNodeData>[] = [
      { id: "n1", type: "golden", position: { x: 0, y: 0 }, data: { nodeType: "compute", label: "App", capability: {}, sizing: {} } },
    ];
    const doc = serialize(nodes, []);
    expect(Object.prototype.hasOwnProperty.call(doc.nodes[0], "sizing")).toBe(false);
  });

  it("includes service_id when the architect picked one (PC-136)", () => {
    const nodes: Node<CanvasNodeData>[] = [
      { id: "n1", type: "golden", position: { x: 0, y: 0 }, data: { nodeType: "managed_database", label: "DB", capability: {}, serviceID: "aws_db_instance" } },
    ];
    const doc = serialize(nodes, []);
    expect(doc.nodes[0].service_id).toBe("aws_db_instance");
  });

  it("preserves directory service labels and core types without inventing provider resource IDs", () => {
    const nodes: Node<CanvasNodeData>[] = [
      { id: "ai", type: "golden", position: { x: 0, y: 0 }, data: { nodeType: "external_dependency", label: "Amazon Bedrock", capability: {}, serviceID: "aws_service:bedrock" } },
    ];
    expect(serialize(nodes, []).nodes[0]).toEqual({ id: "ai", type: "external_dependency", label: "Amazon Bedrock", capability: {} });
  });

  it("omits service_id entirely when no service was picked — never a guessed default", () => {
    const nodes: Node<CanvasNodeData>[] = [
      { id: "n1", type: "golden", position: { x: 0, y: 0 }, data: { nodeType: "managed_database", label: "DB", capability: {} } },
    ];
    const doc = serialize(nodes, []);
    expect(Object.prototype.hasOwnProperty.call(doc.nodes[0], "service_id")).toBe(false);
  });

  it("includes security_group_rules when the architect authored at least one (PC-137)", () => {
    const nodes: Node<CanvasNodeData>[] = [
      {
        id: "sg1",
        type: "golden",
        position: { x: 0, y: 0 },
        data: {
          nodeType: "network_boundary",
          label: "DB SG",
          capability: {},
          securityGroupRules: [{ direction: "ingress", protocol: "tcp", from_port: 5432, to_port: 5432, cidr_blocks: ["0.0.0.0/0"] }],
        },
      },
    ];
    const doc = serialize(nodes, []);
    expect(doc.nodes[0].security_group_rules).toEqual([
      { direction: "ingress", protocol: "tcp", from_port: 5432, to_port: 5432, cidr_blocks: ["0.0.0.0/0"] },
    ]);
  });

  it("omits security_group_rules entirely when none were authored — never an empty array", () => {
    const nodes: Node<CanvasNodeData>[] = [
      { id: "sg1", type: "golden", position: { x: 0, y: 0 }, data: { nodeType: "network_boundary", label: "SG", capability: {} } },
    ];
    const doc = serialize(nodes, []);
    expect(Object.prototype.hasOwnProperty.call(doc.nodes[0], "security_group_rules")).toBe(false);
  });

  it("strips all UI-only fields from edges and maps source/target to from/to", () => {
    const edges: Edge<CanvasEdgeData>[] = [
      {
        id: "e1",
        source: "n1",
        target: "n2",
        sourceHandle: "bottom",
        targetHandle: "top",
        selected: true,
        animated: true,
        label: "reads/writes",
        data: { edgeType: "reads/writes" },
      },
    ];

    const doc = serialize([], edges);

    expect(doc.edges).toEqual([
      { id: "e1", type: "reads/writes", from: "n1", to: "n2" },
    ]);
    const serializedEdge = doc.edges[0] as unknown as Record<string, unknown>;
    for (const uiOnlyField of ["sourceHandle", "targetHandle", "selected", "animated", "label", "source", "target"]) {
      expect(serializedEdge[uiOnlyField]).toBeUndefined();
    }
  });

  it("defaults an edge's type to depends_on if data is somehow missing", () => {
    const edges: Edge<CanvasEdgeData>[] = [
      { id: "e1", source: "n1", target: "n2" },
    ];
    const doc = serialize([], edges);
    expect(doc.edges[0].type).toBe("depends_on");
  });

  it("produces an empty document for an empty canvas", () => {
    const doc = serialize([], []);
    expect(doc).toEqual({ nodes: [], edges: [] });
  });
});
