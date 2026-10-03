import { BaseEdge, getSmoothStepPath, type Edge, type EdgeProps } from "@xyflow/react";
import type { CanvasEdgeData } from "./types";

export function TrafficEdge(props: EdgeProps<Edge<CanvasEdgeData>>) {
  const [path, x, y] = getSmoothStepPath({ ...props, borderRadius: 16 });
  const frame = props.data?.traffic;
  return <>
    <BaseEdge id={props.id} path={path} markerEnd={props.markerEnd} style={props.style} interactionWidth={24}
      label={props.label} labelX={x} labelY={props.id === "traffic-display:dns-lookup" ? y - 48 : frame?.active ? y - 22 : y} labelStyle={props.labelStyle} labelShowBg={props.labelShowBg}
      labelBgStyle={props.labelBgStyle} labelBgPadding={props.labelBgPadding} labelBgBorderRadius={props.labelBgBorderRadius} />
    {frame?.active && (frame.kind === "dns" || frame.allowed ? <circle key={frame.packetKey} r={6} fill={frame.kind === "dns" ? "var(--cyan-ink)" : "var(--purple-ink)"} stroke="white" strokeWidth={2}
      className={`traffic-packet${frame.reducedMotion ? " traffic-packet-static" : ""}`} data-testid="traffic-packet"
      data-traffic-kind={frame.kind ?? "assessed"}
      style={{ offsetPath: `path('${path}')`, offsetDistance: frame.reducedMotion ? "50%" : undefined, animationDuration: `${frame.duration}ms`, animationPlayState: frame.playing ? "running" : "paused" }} />
      : <g transform={`translate(${x}, ${y})`} className="traffic-stop-marker" data-testid="traffic-stop-marker" aria-label="This hop did not pass">
        <circle r={10} fill="var(--warning-soft)" stroke="var(--warning-ink)" strokeWidth={2} />
        <path d="M -3 -3 L 3 3 M -3 3 L 3 -3" stroke="var(--warning-ink)" strokeWidth={2} />
      </g>)}
  </>;
}
