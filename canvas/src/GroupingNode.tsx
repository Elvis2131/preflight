import type { NodeProps, Node } from "@xyflow/react";
import type { GroupingData } from "./groupings";

// GroupingNode (PC-105) draws a Region or AZ box. It is display-only: no handles, no
// interaction (pointer-events are off on the node itself), nothing editable — the box
// follows the subnet attributes the server derived it from. The label says so.
export type GroupingNodeType = Node<GroupingData, "grouping">;

export function GroupingNode({ data }: NodeProps<GroupingNodeType>) {
  const isRegion = data.kind === "region";
  return (
    <div
      data-grouping={data.kind}
      data-label={data.label}
      title={`${isRegion ? "Region" : "Availability Zone"} ${data.label} — derived from subnet attributes; read-only`}
      style={{
        width: "100%",
        height: "100%",
        boxSizing: "border-box",
        borderRadius: 10,
        border: isRegion ? "2px solid #7c3aed" : "1.5px dashed #0369a1",
        background: isRegion ? "rgba(124,58,237,0.04)" : "rgba(3,105,161,0.04)",
        pointerEvents: "none",
        fontSize: 11,
      }}
    >
      <div style={{ position: "absolute", ...(isRegion ? { top: 4 } : { bottom: 4 }), left: 10, color: isRegion ? "#6d28d9" : "#0369a1", fontWeight: 700 }}>
        {isRegion ? "Region" : "AZ"} · {data.label}
        <span style={{ fontWeight: 400, opacity: 0.7 }}> (derived)</span>
      </div>
    </div>
  );
}
