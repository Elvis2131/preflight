import { Handle, Position, type NodeProps, type Node } from "@xyflow/react";
import type { CanvasNodeData } from "./types";
import { NODE_TYPE_LABELS } from "./goldenVocabulary";

// GoldenNode is the ONE custom node component every golden vocabulary type renders
// through — deliberately one component, not eleven. The palette restriction (PC-85's
// own first criterion) is a data/vocabulary decision, not a rendering one; giving
// each node type its own bespoke component would invite exactly the kind of
// per-service special-casing CLAUDE.md's own "giant service conditional" anti-pattern
// warns against, just moved into the canvas instead of the simulator.
export type GoldenNodeType = Node<CanvasNodeData, "golden">;

// simStateBorder/simStateBackground give the three post-simulation states (PC-88)
// visually distinct treatment: "killed" (the node the user targeted) gets a solid red
// border; "severed" (in /simulate's own severed_paths) gets greyed-out fill — this is
// the exact set PC-88's own acceptance criterion says the visual must match; "cascaded"
// (in cascade[] but not severed_paths — e.g. unreachable non-stateful infrastructure)
// gets a lighter grey, kept visually distinct so severed_paths' own boundary stays
// legible rather than blurred into a single "affected" look.
function simStateStyle(simState: string | undefined, selected: boolean | undefined) {
  switch (simState) {
    case "killed":
      return { border: "3px solid #dc2626", background: "#fef2f2" };
    case "severed":
      return { border: "2px dashed #64748b", background: "#e2e8f0", opacity: 0.7 };
    case "cascaded":
      return { border: "1px dashed #94a3b8", background: "#f1f5f9", opacity: 0.85 };
    default:
      return { border: selected ? "2px solid #2563eb" : "1px solid #94a3b8", background: "#fff" };
  }
}

export function GoldenNode({ data, selected }: NodeProps<GoldenNodeType>) {
  const { border, background, opacity } = simStateStyle(data.simState, selected);
  return (
    <div
      style={{
        padding: "8px 12px",
        borderRadius: 6,
        border,
        background,
        opacity,
        minWidth: 140,
        fontSize: 13,
        boxShadow: "0 1px 2px rgba(0,0,0,0.08)",
      }}
    >
      <Handle type="target" position={Position.Top} />
      <div style={{ fontWeight: 600 }}>{data.label}</div>
      <div style={{ color: "#64748b", fontSize: 11 }}>
        {NODE_TYPE_LABELS[data.nodeType]}
      </div>
      {data.simState && data.simState !== "normal" && (
        <div style={{ fontSize: 10, fontWeight: 600, marginTop: 2, color: "#dc2626" }}>
          {data.simState.toUpperCase()}
        </div>
      )}
      <Handle type="source" position={Position.Bottom} />
    </div>
  );
}
