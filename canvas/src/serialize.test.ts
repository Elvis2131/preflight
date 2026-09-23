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
