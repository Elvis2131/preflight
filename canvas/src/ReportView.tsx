import { useEffect, useState } from "react";
import {
  listVersions,
  getReport,
  getPricingSnapshot,
  reportExportURL,
  type Report,
  type PricingSnapshot,
} from "./api";

// ReportView is PC-123's own Report mode: thin UI over PC-120 (JSON) and PC-122
// (HTML/PDF). Every value below is displayed exactly as the server returned it —
// this component computes nothing (no verdict, no diff, no derived number). Version
// comparison shows each selected version's OWN already-server-computed delta side by
// side; it never subtracts or compares two reports against each other client-side
// (see readme note in App.tsx's own Report-mode wiring for the grep/test that proves
// this).

const cardStyle: React.CSSProperties = { border: "1px solid #e2e8f0", borderRadius: 6, padding: 12, marginBottom: 12 };
const notAssessableStyle: React.CSSProperties = { color: "#92400e", fontStyle: "italic" };
const tableStyle: React.CSSProperties = { width: "100%", fontSize: 11, borderCollapse: "collapse" };
const thtdStyle: React.CSSProperties = { border: "1px solid #e2e8f0", padding: "3px 6px", textAlign: "left" };

function ReportSummary({ report, onRepriced }: { report: Report; onRepriced?: () => void }) {
  const [snapshot, setSnapshot] = useState<PricingSnapshot | null>(null);

  useEffect(() => {
    setSnapshot(null);
    const id = report.cost.report?.SnapshotID;
    if (!id) return;
    let cancelled = false;
    getPricingSnapshot(id)
      .then((s) => {
        if (!cancelled) setSnapshot(s);
      })
      .catch(() => {
        /* snapshot metadata is a nicety; the cost figures themselves already rendered */
      });
    return () => {
      cancelled = true;
    };
  }, [report.cost.report?.SnapshotID]);

  return (
    <div>
      <h3 style={{ fontSize: 13, margin: "0 0 6px" }}>
        {report.session_id} — version {report.version_number}
      </h3>

      <div style={cardStyle}>
        <strong>Executive summary</strong>
        <div style={{ fontSize: 11, marginTop: 4 }}>
          NFR: {report.executive_summary.nfr_evaluated_count} evaluated,{" "}
          {report.executive_summary.nfr_not_evaluated_count} not evaluated
        </div>
        <table style={{ ...tableStyle, marginTop: 6 }}>
          <thead>
            <tr>
              <th style={thtdStyle}>Framework</th>
              <th style={thtdStyle}>Assessable</th>
              <th style={thtdStyle}>Partial</th>
              <th style={thtdStyle}>Not assessable</th>
            </tr>
          </thead>
          <tbody>
            {Object.entries(report.executive_summary.compliance_catalog_counts).map(([fw, c]) => (
              <tr key={fw}>
                <td style={thtdStyle}>{fw}</td>
                <td style={thtdStyle}>{c.Assessable}</td>
                <td style={thtdStyle}>{c.Partial}</td>
                <td style={{ ...thtdStyle, ...notAssessableStyle }}>{c.NotAssessable}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div style={cardStyle}>
        <strong>Failure modes</strong>
        {report.failure_modes.available ? (
          <div style={{ fontSize: 11, marginTop: 4 }}>{report.failure_modes.findings?.length ?? 0} finding(s)</div>
        ) : (
          <div style={{ ...notAssessableStyle, fontSize: 11, marginTop: 4 }}>
            Not available: {report.failure_modes.unavailable_reason}
          </div>
        )}
      </div>

      <div style={cardStyle}>
        <strong>Traffic and capacity</strong>
        {report.traffic.available ? (
          <table style={{ ...tableStyle, marginTop: 6 }}>
            <thead>
              <tr>
                <th style={thtdStyle}>Journey</th>
                <th style={thtdStyle}>Flows</th>
                <th style={thtdStyle}>Blocked at</th>
              </tr>
            </thead>
            <tbody>
              {(report.traffic.flows ?? []).map((f) => (
                <tr key={f.JourneyID}>
                  <td style={thtdStyle}>{f.JourneyID}</td>
                  <td style={thtdStyle}>{String(f.Flows)}</td>
                  <td style={{ ...thtdStyle, ...(f.Flows ? {} : notAssessableStyle) }}>{f.BlockedAt ?? ""}</td>
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          <div style={{ ...notAssessableStyle, fontSize: 11, marginTop: 4 }}>
            Not available: {report.traffic.unavailable_reason}
          </div>
        )}
      </div>

      {(report.scenarios ?? []).length > 0 && (
        <div style={cardStyle} data-testid="report-scenarios">
          <strong>Saved failure scenarios</strong>
          <p style={{ fontSize: 10, color: "#64748b", margin: "4px 0" }}>
            Each is re-run against this version by the server — never carried over from an earlier result.
          </p>
          <table style={tableStyle}>
            <thead>
              <tr>
                <th style={thtdStyle}>Scenario</th>
                <th style={thtdStyle}>Verdict</th>
                <th style={thtdStyle}>Failed journeys</th>
                <th style={thtdStyle}>Degraded journeys</th>
              </tr>
            </thead>
            <tbody>
              {(report.scenarios ?? []).map((sc) => (
                <tr key={sc.name}>
                  <td style={thtdStyle}>{sc.name}</td>
                  <td style={{ ...thtdStyle, ...(sc.verdict.state === "assessed" ? {} : notAssessableStyle) }}>
                    {sc.verdict.state === "assessed" ? String(sc.verdict.value) : `not assessable: ${sc.verdict.reason}`}
                  </td>
                  <td style={thtdStyle}>{sc.failed_journeys.join(", ")}</td>
                  <td style={thtdStyle}>{sc.degraded_journeys.join(", ")}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <div style={cardStyle}>
        <strong>Cost</strong>
        <p style={{ fontSize: 10, background: "#fef3c7", border: "1px solid #d97706", padding: 6, margin: "6px 0" }}>
          {report.cost.disclaimer}
        </p>
        {report.cost.available && report.cost.report ? (
          <>
            <div style={{ fontSize: 11 }}>
              Priced total: {report.cost.report.Currency} {report.cost.report.PricedTotal.toFixed(2)} — snapshot{" "}
              <code>{report.cost.report.SnapshotID}</code>
              {snapshot && <> ({new Date(snapshot.fetched_at).toLocaleDateString()})</>}
              {report.cost.report.UnpricedCount > 0 && (
                <span style={notAssessableStyle}> — {report.cost.report.UnpricedCount} unpriced component(s)</span>
              )}
            </div>
            <table style={{ ...tableStyle, marginTop: 6 }} data-testid="cost-components">
              <thead>
                <tr>
                  <th style={thtdStyle}>Component</th>
                  <th style={thtdStyle}>Monthly</th>
                  <th style={thtdStyle}>Region</th>
                  <th style={thtdStyle}>Note</th>
                </tr>
              </thead>
              <tbody>
                {report.cost.report.Components.filter((c) => c.Decision === "priced" || c.Reason.includes("region")).map((c) => (
                  <tr key={c.NodeID}>
                    <td style={thtdStyle}>{c.NodeID}</td>
                    <td style={{ ...thtdStyle, ...(c.Decision === "priced" ? {} : notAssessableStyle) }}>
                      {c.Decision === "priced" ? `${c.Currency} ${c.MonthlyAmount.toFixed(2)}` : "unpriced"}
                    </td>
                    <td style={thtdStyle}>{c.Region ? `${c.Region} (${c.RegionSource === "component" ? "component's own" : "workload's only"})` : ""}</td>
                    <td style={{ ...thtdStyle, ...(c.Decision === "priced" ? {} : notAssessableStyle) }}>{c.Decision === "priced" ? "" : c.Reason}</td>
                  </tr>
                ))}
              </tbody>
            </table>
            {onRepriced && (
              <button style={{ fontSize: 11, marginTop: 6 }} onClick={onRepriced}>
                Re-price with latest snapshot (creates a new version)
              </button>
            )}
          </>
        ) : (
          <div style={notAssessableStyle}>Not available: {report.cost.unavailable_reason}</div>
        )}
      </div>

      <div style={cardStyle}>
        <strong>Change since previous version</strong>
        {report.delta.length > 0 ? (
          <table style={{ ...tableStyle, marginTop: 6 }}>
            <thead>
              <tr>
                <th style={thtdStyle}>Finding</th>
                <th style={thtdStyle}>Kind</th>
              </tr>
            </thead>
            <tbody>
              {report.delta.map((d) => (
                <tr key={d.finding_id}>
                  <td style={thtdStyle}>{d.finding_id}</td>
                  <td style={thtdStyle}>{d.kind}</td>
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          <div style={{ fontSize: 11, color: "#64748b" }}>No previous version to compare against, or nothing changed.</div>
        )}
      </div>

      <div style={cardStyle}>
        <strong>Assumptions appendix</strong>
        {report.assumptions.length > 0 ? (
          <ul style={{ fontSize: 11, margin: "6px 0 0", paddingLeft: 18 }}>
            {report.assumptions.map((a, i) => (
              <li key={i}>
                <strong>{a.kind}</strong>: {a.source} {a.reason && <span style={{ color: "#64748b" }}>— {a.reason}</span>}
              </li>
            ))}
          </ul>
        ) : (
          <div style={{ fontSize: 11, color: "#64748b", marginTop: 4 }}>
            No assumed or stated values are present in this report's own computed sections.
          </div>
        )}
      </div>

      <div style={{ display: "flex", gap: 8 }}>
        <a href={reportExportURL(report.session_id, report.version_number, "html")} target="_blank" rel="noreferrer">
          <button style={{ fontSize: 11 }}>Export HTML</button>
        </a>
        <a href={reportExportURL(report.session_id, report.version_number, "pdf")} target="_blank" rel="noreferrer">
          <button style={{ fontSize: 11 }}>Export PDF</button>
        </a>
      </div>
    </div>
  );
}

export function ReportView({
  sessionID,
  onReprice,
}: {
  sessionID: string;
  // onReprice re-runs assessCanvas against the CURRENT canvas/workload state (App.tsx
  // owns that state, not this component — Report mode only ever displays already-
  // computed results) and returns the new version number it created.
  onReprice: () => Promise<number>;
}) {
  const [versions, setVersions] = useState<number[]>([]);
  const [selectedA, setSelectedA] = useState<number | null>(null);
  const [selectedB, setSelectedB] = useState<number | null>(null);
  const [compareMode, setCompareMode] = useState(false);
  const [reportA, setReportA] = useState<Report | null>(null);
  const [reportB, setReportB] = useState<Report | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const loadVersions = () => {
    listVersions(sessionID)
      .then((list) => {
        const nums = list.map((v) => v.version_number).sort((a, b) => b - a);
        setVersions(nums);
        if (nums.length > 0 && selectedA === null) setSelectedA(nums[0]);
      })
      .catch((err) => setError(err instanceof Error ? err.message : String(err)));
  };

  useEffect(loadVersions, [sessionID]);

  const generate = async () => {
    setError(null);
    setBusy(true);
    try {
      if (selectedA !== null) setReportA(await getReport(sessionID, selectedA));
      if (compareMode && selectedB !== null) setReportB(await getReport(sessionID, selectedB));
      else setReportB(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  const reprice = async () => {
    setBusy(true);
    setError(null);
    try {
      const newVersion = await onReprice();
      loadVersions();
      setSelectedA(newVersion);
      setReportA(await getReport(sessionID, newVersion));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div style={{ padding: 12, fontSize: 12, overflowY: "auto", height: "100%" }}>
      <h2 style={{ fontSize: 15, margin: "0 0 8px" }}>Report</h2>
      <div style={{ display: "flex", gap: 8, alignItems: "center", marginBottom: 10, flexWrap: "wrap" }}>
        <label>
          Version:{" "}
          <select value={selectedA ?? ""} onChange={(e) => setSelectedA(Number(e.target.value))}>
            {versions.map((v) => (
              <option key={v} value={v}>
                {v}
              </option>
            ))}
          </select>
        </label>
        <label>
          <input type="checkbox" checked={compareMode} onChange={(e) => setCompareMode(e.target.checked)} /> Compare
          with
        </label>
        {compareMode && (
          <select value={selectedB ?? ""} onChange={(e) => setSelectedB(Number(e.target.value))}>
            <option value="">— select —</option>
            {versions
              .filter((v) => v !== selectedA)
              .map((v) => (
                <option key={v} value={v}>
                  {v}
                </option>
              ))}
          </select>
        )}
        <button onClick={generate} disabled={busy || selectedA === null}>
          {busy ? "Loading..." : "Generate / view report"}
        </button>
      </div>

      {error && <p style={{ color: "#dc2626", fontSize: 11 }}>Error: {error}</p>}

      {reportA && !reportB && <ReportSummary report={reportA} onRepriced={reprice} />}

      {reportA && reportB && (
        <div style={{ display: "flex", gap: 16 }}>
          <div style={{ flex: 1 }}>
            <ReportSummary report={reportA} />
          </div>
          <div style={{ flex: 1 }}>
            <ReportSummary report={reportB} />
          </div>
        </div>
      )}
    </div>
  );
}
