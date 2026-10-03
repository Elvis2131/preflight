import { describe, expect, it } from "vitest";
import type { Node } from "@xyflow/react";
import type { CanvasNodeData } from "./types";
import { applyCanvasNodeChanges } from "./containerResize";
import { innermostContainer } from "./containment";
import { serialize } from "./serialize";

const container = (id: string, serviceID: string, width: number, height: number): Node<CanvasNodeData> => ({
  id, position: { x: 0, y: 0 }, style: { width, height }, measured: { width, height },
  data: { nodeType: "network_boundary", label: id, serviceID, capability: {} },
});

describe("container resizing", () => {
  it.each([
    { parentService: "aws_vpc", childService: "aws_subnet", width: 560, height: 380 },
    { parentService: "aws_subnet", childService: "aws_instance", width: 260, height: 190 },
  ])("new drop geometry uses resized $parentService bounds, while assessment input is unchanged", ({ parentService, childService, width, height }) => {
    const parent = container("parent", parentService, width, height);
    const child = { ...container("child", childService, 220, 100), position: { x: width + 10, y: height + 10 } };
    const before = serialize([parent, child], []);
    expect(innermostContainer(child, [parent, child])).toBeNull();
    const resized = applyCanvasNodeChanges([{ id: parent.id, type: "dimensions", dimensions: { width: width + 400, height: height + 300 }, resizing: true, setAttributes: true }], [parent, child]);
    expect(innermostContainer(child, resized)).toBe(parent.id);
    expect(serialize(resized, [])).toEqual(before);
    expect(innermostContainer(child, [parent, child])).toBeNull(); // Original state is immutable.
  });

  it("ordinary measurement notifications do not replace an explicit container size", () => {
    const parent = container("vpc", "aws_vpc", 560, 380);
    const [measured] = applyCanvasNodeChanges([{ id: parent.id, type: "dimensions", dimensions: { width: 150, height: 60 } }], [parent]);
    const child = { ...container("subnet", "aws_subnet", 260, 190), position: { x: 20, y: 60 } };
    expect(innermostContainer(child, [measured, child])).toBe(parent.id);
    expect(measured.style).toEqual(parent.style);
  });
});
