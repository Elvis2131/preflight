import { Handle, Position, type NodeProps, type Node } from "@xyflow/react";
import type { CanvasNodeData } from "./types";
import { NODE_TYPE_LABELS } from "./goldenVocabulary";
import { containerRank } from "./containment";
import { AwsIcon } from "./AwsIcon";
import { iconForService, groupIcon, labelForService } from "./awsIcons";

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

// utilizationColor picks a display band purely from the number /simulate already
// computed (core.ComponentLoad.Utilization) — never a threshold this app invents new
// meaning for, just a stoplight rendering of a ratio the server itself produced:
// green under 70%, amber up to 100%, red once a component is offered more load than
// its declared capacity.
function utilizationColor(utilization: number): string {
  if (utilization > 1) return "#dc2626";
  if (utilization >= 0.7) return "#d97706";
  return "#16a34a";
}

export function GoldenNode({ data, selected }: NodeProps<GoldenNodeType>) {
  const { border, background, opacity } = simStateStyle(data.simState, selected);
  // A VPC/subnet is drawn as a container (PC-105): a translucent dashed box that fills
  // the node's own width/height, so resources can be dropped inside it.
  const rank = containerRank(data.serviceID);
  if (rank > 0 && (!data.simState || data.simState === "normal")) {
    return (
      <div
        data-container={data.serviceID}
        style={{
          width: "100%",
          height: "100%",
          boxSizing: "border-box",
          borderRadius: 8,
          border: `2px dashed ${selected ? "#2563eb" : rank === 1 ? "#0f766e" : "#64748b"}`,
          background: rank === 1 ? "rgba(20,184,166,0.06)" : "rgba(100,116,139,0.08)",
          padding: "6px 10px",
          fontSize: 12,
          position: "relative",
        }}
      >
        <Handle type="target" position={Position.Top} />
        <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
          {containerIcon(rank, data) && <AwsIcon src={containerIcon(rank, data)!} size={20} />}
          <div style={{ fontWeight: 600 }}>{data.label}</div>
        </div>
        <div style={{ color: "#64748b", fontSize: 11 }}>
          {rank === 1 ? "VPC" : `Subnet${data.availabilityZone ? ` · ${data.availabilityZone}` : ""}`}
          {data.cidrBlock ? ` · ${data.cidrBlock}` : ""}
        </div>
        {rank === 2 && data.subnetFact && <SubnetBadge fact={data.subnetFact} />}
        <Handle type="source" position={Position.Bottom} />
      </div>
    );
  }
  // journeyOnPath (PC-127) is a highlight ADDED via box-shadow, never a border
  // override: PC-88/89's own killed/severed/cascaded border colors are the one true
  // fault signal (see simStateStyle's own doc comment on why they must stay
  // legible) — a journey highlight must never mask that a node is also killed.
  const journeyRing = data.journeyOnPath ? "0 0 0 3px #7c3aed" : undefined;
  const baseShadow = journeyRing ?? "0 1px 2px rgba(0,0,0,0.08)";
  // notAssessableLoad (PC-127): a diagonal hatch, distinct from any simState color,
  // for a component core.ComponentLoad itself marked not_assessable (no declared
  // capacity) — never silently shown as 0% utilized.
  const hatch = data.notAssessableLoad
    ? {
        backgroundImage:
          "repeating-linear-gradient(45deg, #e2e8f0, #e2e8f0 4px, #f8fafc 4px, #f8fafc 8px)",
      }
    : {};
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
        boxShadow: baseShadow,
        ...hatch,
      }}
    >
      <Handle type="target" position={Position.Top} />
      <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
        {iconForService(data.serviceID) && (
          <AwsIcon src={iconForService(data.serviceID)!} title={data.serviceID ? labelForService(data.serviceID) : undefined} />
        )}
        <div>
          <div style={{ fontWeight: 600 }}>{data.label}</div>
          <div style={{ color: "#64748b", fontSize: 11 }}>{NODE_TYPE_LABELS[data.nodeType]}</div>
        </div>
      </div>
      {data.simState && data.simState !== "normal" && (
        <div style={{ fontSize: 10, fontWeight: 600, marginTop: 2, color: "#dc2626" }}>
          {data.simState.toUpperCase()}
        </div>
      )}
      {typeof data.utilization === "number" && (
        <div style={{ marginTop: 4, height: 4, borderRadius: 2, background: "#e2e8f0" }}>
          <div
            style={{
              width: `${Math.min(data.utilization, 1) * 100}%`,
              height: "100%",
              borderRadius: 2,
              background: utilizationColor(data.utilization),
            }}
          />
        </div>
      )}
      {data.notAssessableLoad && (
        <div style={{ fontSize: 9, color: "#94a3b8", marginTop: 2 }}>utilization not_assessable</div>
      )}
      <Handle type="source" position={Position.Bottom} />
    </div>
  );
}

// SubnetBadge (PC-105) shows a subnet's public/private classification. It is a DERIVED badge:
// the server decides it from the subnet's route table (a route to an internet gateway makes
// it public — AWS's own definition) and sends the answer; nothing is authored or computed
// here. "not assessable" means the subnet has no resolvable route table — never shown as
// private.
function SubnetBadge({ fact }: { fact: NonNullable<CanvasNodeData["subnetFact"]> }) {
  const style =
    fact.visibility === "public"
      ? { background: "#dcfce7", color: "#166534", border: "1px solid #86efac" }
      : fact.visibility === "private"
        ? { background: "#e0e7ff", color: "#3730a3", border: "1px solid #a5b4fc" }
        : { background: "#f1f5f9", color: "#64748b", border: "1px dashed #94a3b8" };
  return (
    <span
      data-testid="subnet-badge"
      data-visibility={fact.visibility}
      title={fact.reason}
      // Top-right of the subnet: the resources drawn inside sit under the header on the left,
      // and a badge in the flow of the text would be covered by them.
      style={{ ...style, position: "absolute", top: 6, right: 8, padding: "0 6px", borderRadius: 8, fontSize: 10, fontWeight: 600 }}
    >
      {fact.visibility === "not_assessable" ? "not assessable" : fact.visibility}
    </span>
  );
}

// containerIcon picks a VPC/subnet's group icon. A subnet's public/private icon follows the
// SERVER-derived classification (the same source as the badge) — a subnet whose visibility is
// unknown (no route table) gets no icon rather than a guess.
function containerIcon(rank: number, data: CanvasNodeData): string | null {
  if (rank === 1) return groupIcon("vpc");
  if (data.subnetFact?.visibility === "public") return groupIcon("publicSubnet");
  if (data.subnetFact?.visibility === "private") return groupIcon("privateSubnet");
  return null;
}
