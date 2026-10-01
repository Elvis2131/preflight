import { useState, useEffect } from "react";
import { listFullVersions, type AssessResponse } from "./api";
import { buildTimeline } from "./timeline";
import { GraphView } from "./GraphView";

// AnalyzeView (PC-104) is the former standalone viewer (PC-90/91/92) as a workspace mode:
// per-finding status across every stored version of THIS session, coloured by the
// server's own assurance-delta classification, plus the server-rendered graph and diff
// for one version. It authors nothing and computes nothing — no write/edit action
// anywhere, no average, no trend line, no composite score (CLAUDE.md §10). It follows
// the workspace's session, so switching modes never loses which design is being looked at;
// `latestVersion` changing (a new assessment ran in another mode) reloads it.

const KIND_COLOR: Record<string, string> = {
  improvement: "#166534",
  resolved_risk: "#0e7490",
  regression: "#b91c1c",
  new_risk: "#b91c1c",
  unchanged: "#64748b",
  not_assessable: "#94a3b8",
};

function StatusCell({ status, deltaKind }: { status: string | null; deltaKind?: string }) {
  if (status === null) {
    return <td style={{ padding: "4px 8px", color: "#cbd5e1", fontSize: 12 }}>—</td>;
  }
  const color = deltaKind ? KIND_COLOR[deltaKind] : "#334155";
  return (
    <td style={{ padding: "4px 8px", fontSize: 12, borderLeft: "1px solid #f1f5f9" }}>
      <div style={{ color, fontWeight: deltaKind && deltaKind !== "unchanged" ? 700 : 400 }}>{status}</div>
      {deltaKind && <div style={{ fontSize: 10, color, opacity: 0.8 }}>{deltaKind}</div>}
    </td>
  );
}

export function AnalyzeView({ sessionID, latestVersion }: { sessionID: string; latestVersion: number | null }) {
  const [versions, setVersions] = useState<AssessResponse[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [selectedVersion, setSelectedVersion] = useState<number | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (latestVersion === null) {
      setVersions(null);
      return;
    }
    let cancelled = false;
    setBusy(true);
    setError(null);
    listFullVersions(sessionID)
      .then((v) => {
        if (cancelled) return;
        setVersions(v);
        // Default to the newest version — "for a given version", never every one at once.
        setSelectedVersion(v.length > 0 ? v[v.length - 1].version_number : null);
      })
      .catch((err) => !cancelled && setError(err instanceof Error ? err.message : String(err)))
      .finally(() => !cancelled && setBusy(false));
    return () => {
      cancelled = true;
    };
  }, [sessionID, latestVersion]);

  if (latestVersion === null) {
    return (
      <div className="mode-empty" style={{ padding: 16, fontSize: 13, color: "#697386" }} data-testid="analyze-empty">
        Nothing to analyse yet — this session has no assessed version. Run a baseline in Simulate or inject a fault
        in Failure Lab (each assesses the current design), then come back.
      </div>
    );
  }

  const rows = versions ? buildTimeline(versions) : null;
  const current = versions?.find((x) => x.version_number === selectedVersion) ?? null;

  return (
    <div className="evidence-view" style={{ padding: 24, fontSize: 13, maxWidth: 1100, margin: "0 auto", overflowY: "auto", flex: 1 }}>
      <h1 style={{ fontSize: 18, margin: "0 0 4px" }}>Analyze — scorecard timeline</h1>
      <p style={{ fontSize: 12, color: "#64748b", margin: "0 0 16px" }}>
        Read-only. Per-finding status across every stored version of this session, coloured by the server's own
        assurance-delta classification — nothing here is combined into a single trend line or score.
        {busy && " Loading…"}
      </p>
      {error && <div className="mode-error" style={{ padding: 10, background: "#fff5f5", color: "#a83c3c", fontSize: 12, marginBottom: 12 }}>{error}</div>}

      {rows && versions && versions.length > 0 && (
        <div style={{ overflowX: "auto" }}>
          <table style={{ borderCollapse: "collapse", width: "100%" }} data-testid="timeline">
            <thead>
              <tr>
                <th style={{ textAlign: "left", padding: "4px 8px", fontSize: 12, borderBottom: "2px solid #e2e8f0" }}>finding_id</th>
                {versions.map((v) => (
                  <th key={v.version_number} style={{ textAlign: "left", padding: "4px 8px", fontSize: 12, borderBottom: "2px solid #e2e8f0" }}>
                    V{v.version_number}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => (
                <tr key={row.findingID}>
                  <td style={{ padding: "4px 8px", fontSize: 11, fontFamily: "monospace", color: "#334155" }}>{row.findingID}</td>
                  {row.cells.map((cell, i) => (
                    <StatusCell key={i} status={cell.status} deltaKind={cell.deltaKind} />
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <div style={{ marginTop: 16, fontSize: 11, color: "#64748b" }}>
        <strong>Legend:</strong>{" "}
        {Object.entries(KIND_COLOR).map(([k, c]) => (
          <span key={k} style={{ marginRight: 12, color: c }}>
            ● {k}
          </span>
        ))}
      </div>

      {versions && versions.length > 0 && (
        <div style={{ marginTop: 32, borderTop: "1px solid #e2e8f0", paddingTop: 16 }}>
          <label style={{ display: "block", marginBottom: 12 }}>
            graph version:{" "}
            <select value={selectedVersion ?? ""} onChange={(e) => setSelectedVersion(Number(e.target.value))} style={{ fontSize: 12 }}>
              {versions.map((v) => (
                <option key={v.version_number} value={v.version_number}>
                  V{v.version_number}
                </option>
              ))}
            </select>
          </label>
          {current && <GraphView version={current} />}
        </div>
      )}
    </div>
  );
}
