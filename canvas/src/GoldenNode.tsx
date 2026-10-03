import { Handle, NodeResizer, Position, type NodeProps, type Node } from "@xyflow/react";
import type { CanvasNodeData } from "./types";
import { NODE_TYPE_LABELS } from "./goldenVocabulary";
import { containerRank } from "./containment";
import { AwsIcon } from "./AwsIcon";
import { groupIcon, labelForService } from "./awsIcons";
import { ServiceMark } from "./ServiceMark";

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
      return { border: "3px solid var(--danger-ink)", background: "var(--danger-soft)" };
    case "severed":
      return { border: "2px dashed var(--muted)", background: "var(--line)", opacity: 0.7 };
    case "cascaded":
      return { border: "1px dashed var(--subtle)", background: "var(--surface-soft)", opacity: 0.85 };
    default:
      return { border: selected ? "2px solid var(--accent)" : "1px solid var(--line)", background: "var(--surface)" };
  }
}

// utilizationColor picks a display band purely from the number /simulate already
// computed (core.ComponentLoad.Utilization) — never a threshold this app invents new
// meaning for, just a stoplight rendering of a ratio the server itself produced:
// green under 70%, amber up to 100%, red once a component is offered more load than
// its declared capacity.
function utilizationColor(utilization: number): string {
  if (utilization > 1) return "var(--danger-ink)";
  if (utilization >= 0.7) return "var(--warning-ink)";
  return "var(--success-ink)";
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
        data-service-id={data.serviceID}
        style={{
          width: "100%",
          height: "100%",
          boxSizing: "border-box",
          borderRadius: 8,
          border: `2px dashed ${selected ? "var(--accent)" : rank === 1 ? "#0f766e" : "var(--muted)"}`,
          background: rank === 1 ? "rgba(20,184,166,0.06)" : "rgba(100,116,139,0.08)",
          padding: "6px 10px",
          fontSize: 12,
          position: "relative",
        }}
      >
        <NodeResizer isVisible={!!selected && !!data.canResizeContainer} color={rank === 1 ? "#0f766e" : "var(--accent)"}
          minWidth={rank === 1 ? 320 : 240} minHeight={rank === 1 ? 200 : 140}
          handleClassName="container-resize-handle" lineClassName="container-resize-line" />
        <Handle type="target" position={Position.Top} />
        <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
          {containerIcon(rank, data) && <AwsIcon src={containerIcon(rank, data)!} size={20} />}
          <div style={{ fontWeight: 600 }}>{data.label}</div>
        </div>
        <div style={{ color: "var(--muted)", fontSize: 11 }}>
          {rank === 1 ? "VPC" : `Subnet${data.availabilityZone ? ` · ${data.availabilityZone}` : ""}`}
          {data.cidrBlock ? ` · ${data.cidrBlock}` : ""}
        </div>
        {rank === 2 && data.subnetFact && <SubnetBadge fact={data.subnetFact} />}
        {selected && data.canResizeContainer && <span className="container-resize-hint">Drag a corner or border to resize</span>}
        <Handle type="source" position={Position.Bottom} />
      </div>
    );
  }
  // journeyOnPath (PC-127) is a highlight ADDED via box-shadow, never a border
  // override: PC-88/89's own killed/severed/cascaded border colors are the one true
  // fault signal (see simStateStyle's own doc comment on why they must stay
  // legible) — a journey highlight must never mask that a node is also killed.
  const journeyRing = data.journeyOnPath ? "0 0 0 3px var(--purple-ink)" : undefined;
  const baseShadow = journeyRing ?? "0 2px 0 var(--line), 0 3px 5px rgba(24,32,51,0.04)";
  // notAssessableLoad (PC-127): a diagonal hatch, distinct from any simState color,
  // for a component core.ComponentLoad itself marked not_assessable (no declared
  // capacity) — never silently shown as 0% utilized.
  const hatch = data.notAssessableLoad
    ? {
        backgroundImage:
          "repeating-linear-gradient(45deg, var(--line), var(--line) 4px, var(--surface-soft) 4px, var(--surface-soft) 8px)",
      }
    : {};
  return (
    <div
      data-service-id={data.serviceID}
      className={`service-node${data.trafficStep ? " traffic-active-node" : ""}`}
      data-traffic-step={data.trafficStep}
      style={{
        padding: "0",
        borderRadius: 10,
        border,
        background,
        opacity: data.trafficDimmed && (!data.simState || data.simState === "normal") ? 0.4 : opacity,
        width: 220,
        boxSizing: "border-box",
        fontSize: 13,
        boxShadow: baseShadow,
        position: "relative",
        ...hatch,
      }}
    >
      <Handle type="target" position={Position.Top} />
      {data.trafficStep && <span className="traffic-node-step" aria-label={`Traffic hop ${data.trafficStep}`}>{data.trafficStep}</span>}
      <div className="service-node-type">{NODE_TYPE_LABELS[data.nodeType]}</div>
      <div className="service-node-body" style={{ display: "flex", alignItems: "center", gap: 10 }}>
        {data.serviceID && <ServiceMark serviceID={data.serviceID} />}
        <div>
          <div style={{ fontWeight: 600, overflowWrap: "anywhere", lineHeight: 1.4 }}>{data.label}</div>
          <div style={{ color: "var(--muted)", fontSize: 11, marginTop: 2 }}>{data.serviceID ? labelForService(data.serviceID) : NODE_TYPE_LABELS[data.nodeType]}</div>
        </div>
      </div>
      {!!data.configurationSummary?.length && (
        <div className="node-configuration-summary" title="Edit these settings in the service inspector">{data.configurationSummary.join(" · ")}</div>
      )}
      {data.simState && data.simState !== "normal" && (
        <div style={{ fontSize: 10, fontWeight: 600, marginTop: 2, color: "var(--danger-ink)" }}>
          {data.simState.toUpperCase()}
        </div>
      )}
      {typeof data.utilization === "number" && (
        <div style={{ marginTop: 4, height: 4, borderRadius: 2, background: "var(--line)" }}>
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
        <div style={{ fontSize: 9, color: "var(--subtle)", marginTop: 2 }}>utilization not_assessable</div>
      )}
      <Handle type="source" position={Position.Bottom} />
      {data.trafficExternal && <Handle type="source" position={Position.Right} id="dns-query" />}
      {data.serviceID === "aws_route53_record" && <Handle type="target" position={Position.Left} id="dns-query" />}
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
      ? { background: "var(--success-soft)", color: "var(--success-ink)", border: "1px solid var(--success-ink)" }
      : fact.visibility === "private"
        ? { background: "var(--accent-soft)", color: "var(--accent)", border: "1px solid var(--accent-line)" }
        : { background: "var(--surface-soft)", color: "var(--muted)", border: "1px dashed var(--subtle)" };
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
