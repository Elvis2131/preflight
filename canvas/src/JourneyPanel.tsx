import type { DeclaredJourney } from "./workloadTypes";
import type { JourneyFlowResult, ComponentLoad, JourneyLatency, JourneyLatencyEstimate } from "./api";

// JourneyPanel (PC-127) is a thin, read-only view over /simulate's own
// flow_detail/load — same discipline as ReportView.tsx (PC-123): every number and
// verdict shown here is exactly what the server returned, never recomputed,
// reinterpreted, or summarised differently. The panel's only job is picking which
// declared journey to show and mapping the server's already-computed Utilization
// ratio to a display band (see GoldenNode.tsx's own utilizationColor, same rule
// applied to the table below).
export interface JourneyPanelProps {
  journeys: DeclaredJourney[];
  flowDetail: JourneyFlowResult[];
  load: ComponentLoad[];
  latency?: JourneyLatency[];
  selectedJourneyID: string | null;
  onSelectJourney: (id: string) => void;
}

export function JourneyPanel({ journeys, flowDetail, load, latency, selectedJourneyID, onSelectJourney }: JourneyPanelProps) {
  if (journeys.length === 0) {
    return (
      <div className="result-panel" style={{ padding: 16, fontSize: 12, color: "var(--muted)" }}>
        No journeys declared — add one in the Workload form to see traffic flow and
        utilization here.
      </div>
    );
  }

  const flow = flowDetail.find((f) => f.JourneyID === selectedJourneyID) ?? null;
  const lat = latency?.find((l) => l.journey_id === selectedJourneyID)?.result ?? null;
  const latEstimate = lat?.state === "assessed" ? (lat.value as JourneyLatencyEstimate) : null;

  return (
    <div className="result-panel" style={{ padding: 16, fontSize: 12, overflowY: "auto" }}>
      <h3 style={{ fontSize: 13, margin: "0 0 8px" }}>Journey flow &amp; utilization</h3>
      <label style={{ display: "block", marginBottom: 8 }}>
        Journey:{" "}
        <select value={selectedJourneyID ?? ""} onChange={(e) => onSelectJourney(e.target.value)}>
          <option value="" disabled>
            select a journey
          </option>
          {journeys.map((j) => (
            <option key={j.id} value={j.id}>
              {j.name}
            </option>
          ))}
        </select>
      </label>

      {!flow && (
        <p style={{ color: "var(--subtle)" }}>
          Run a simulation (baseline or a fault) to see this journey's flow.
        </p>
      )}

      {flow && (
        <div style={{ marginBottom: 12 }}>
          <div style={{ fontWeight: 600, color: flow.Flows ? "var(--success-ink)" : flow.Degraded ? "var(--warning-ink)" : "var(--danger-ink)" }}>
            {flow.Flows ? "Flows end-to-end" : `Blocked at ${flow.BlockedAt}: ${flow.BlockedReason}`}
          </div>
          {flow.Degraded && (
            <div style={{ color: "var(--warning-ink)", fontSize: 12 }}>
              Degraded, not failed — the declared fallback flows: {flow.DegradedVia}
            </div>
          )}
          <table style={{ width: "100%", marginTop: 6, borderCollapse: "collapse" }}>
            <thead>
              <tr style={{ textAlign: "left", color: "var(--muted)" }}>
                <th>hop</th>
                <th>allowed</th>
                <th>reason</th>
              </tr>
            </thead>
            <tbody>
              {flow.Hops.map((hop, i) => (
                <tr key={i} style={{ borderTop: "1px solid var(--line)" }}>
                  <td>
                    {hop.From} &rarr; {hop.To}
                  </td>
                  <td style={{ color: hop.Allowed ? "var(--success-ink)" : "var(--danger-ink)" }}>{hop.Allowed ? "yes" : "no"}</td>
                  <td style={{ color: "var(--muted)" }}>{hop.Reason}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {flow && lat && (
        <div style={{ marginBottom: 12 }} data-testid="latency">
          <div style={{ fontWeight: 600 }}>Latency (Layer 3)</div>
          {latEstimate ? (
            <>
              <div>{latEstimate.statement}</div>
              <div style={{ color: "var(--muted)" }}>
                Inputs: {latEstimate.inputs.map((i) => `${i.name} = ${i.value} (${i.source})`).join("; ")}
              </div>
              <div style={{ color: "var(--muted)" }}>Assumed: {latEstimate.assumptions.join("; ")}</div>
            </>
          ) : (
            <div style={{ color: "var(--subtle)" }}>Not assessable: {lat.reason}</div>
          )}
        </div>
      )}


      <h4 style={{ fontSize: 12, margin: "8px 0 4px" }}>Component load</h4>
      {load.length === 0 ? (
        <p style={{ color: "var(--subtle)" }}>No component currently carries any declared journey's load.</p>
      ) : (
        <table style={{ width: "100%", borderCollapse: "collapse" }}>
          <thead>
            <tr style={{ textAlign: "left", color: "var(--muted)" }}>
              <th>node</th>
              <th>offered rps</th>
              <th>utilization</th>
            </tr>
          </thead>
          <tbody>
            {load.map((l) => (
              <tr key={l.NodeID} style={{ borderTop: "1px solid var(--line)" }}>
                <td>{l.NodeID}</td>
                <td>{l.OfferedRPS}</td>
                <td>
                  {l.Utilization === null ? (
                    <span style={{ color: "var(--subtle)" }}>not_assessable{l.NotAssessableReason ? ` (${l.NotAssessableReason})` : ""}</span>
                  ) : (
                    `${Math.round(l.Utilization * 100)}%`
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
