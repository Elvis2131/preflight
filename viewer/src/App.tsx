import { useState, useCallback } from "react";
import { listVersions, type AssessResponse } from "./api";
import { buildTimeline } from "./timeline";

// This is PC-90's own read-only viewer (PRD §6) — it authors nothing. There is no
// write/edit action anywhere in this file, deliberately: no rename, no re-run, no
// "fix this finding" button, even a small tempting one. If this ever grows an edit
// affordance, that is PC-84's (the canvas's) territory, not this one's.

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
      {deltaKind && (
        <div style={{ fontSize: 10, color, opacity: 0.8 }}>{deltaKind}</div>
      )}
    </td>
  );
}

export default function App() {
  const [baseURL, setBaseURL] = useState("http://localhost:8080");
  const [sessionID, setSessionID] = useState("");
  const [versions, setVersions] = useState<AssessResponse[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    if (!sessionID.trim()) return;
    setBusy(true);
    setError(null);
    try {
      const v = await listVersions(sessionID.trim(), baseURL.trim());
      setVersions(v);
    } catch (err) {
      setVersions(null);
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }, [sessionID, baseURL]);

  const rows = versions ? buildTimeline(versions) : null;

  return (
    <div style={{ padding: 16, fontSize: 13, maxWidth: 1100, margin: "0 auto" }}>
      <h1 style={{ fontSize: 18, margin: "0 0 4px" }}>Preflight — scorecard timeline</h1>
      <p style={{ fontSize: 12, color: "#64748b", margin: "0 0 16px" }}>
        Read-only (PRD §6). Per-finding status across every stored version, colored by
        the server's own real assurance-delta classification — nothing here is
        combined into a single trend line or score.
      </p>

      <div style={{ display: "flex", gap: 8, marginBottom: 12, alignItems: "center" }}>
        <label>
          assessd URL:{" "}
          <input value={baseURL} onChange={(e) => setBaseURL(e.target.value)} style={{ width: 220, fontSize: 12 }} />
        </label>
        <label>
          session_id:{" "}
          <input
            value={sessionID}
            onChange={(e) => setSessionID(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && load()}
            style={{ width: 220, fontSize: 12 }}
          />
        </label>
        <button onClick={load} disabled={busy || !sessionID.trim()}>
          {busy ? "Loading…" : "Load"}
        </button>
      </div>

      {error && (
        <div style={{ padding: 8, background: "#fef2f2", color: "#b91c1c", fontSize: 12, marginBottom: 12 }}>
          {error}
        </div>
      )}

      {versions && versions.length === 0 && (
        <p style={{ color: "#64748b" }}>This session exists but has no stored versions yet.</p>
      )}

      {rows && versions && versions.length > 0 && (
        <div style={{ overflowX: "auto" }}>
          <table style={{ borderCollapse: "collapse", width: "100%" }}>
            <thead>
              <tr>
                <th style={{ textAlign: "left", padding: "4px 8px", fontSize: 12, borderBottom: "2px solid #e2e8f0" }}>
                  finding_id
                </th>
                {versions.map((v) => (
                  <th
                    key={v.version_number}
                    style={{ textAlign: "left", padding: "4px 8px", fontSize: 12, borderBottom: "2px solid #e2e8f0" }}
                  >
                    V{v.version_number}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => (
                <tr key={row.findingID}>
                  <td style={{ padding: "4px 8px", fontSize: 11, fontFamily: "monospace", color: "#334155" }}>
                    {row.findingID}
                  </td>
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
    </div>
  );
}
