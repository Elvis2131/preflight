import type { AssessResponse } from "./api";

// GraphView (PC-91) renders exactly one version's `graph` field verbatim — the
// server's own real SVG (PC-81) — embedded via dangerouslySetInnerHTML rather than
// through an <img>/data-URI round trip, so nothing here transforms it in any way.
// This is this ticket's own first acceptance criterion, verbatim: "Renders PC-81's
// SVG output for a given version without modification." This is trusted content: the
// SVG comes from assessd, a server this viewer is explicitly pointed at by the
// person running it, not arbitrary third-party/user-uploaded input — the same trust
// boundary every other field in this response already crosses.
//
// The diff below reads the EXISTING assurance_delta array (PC-19) directly off
// `version` — no new diff computation happens in this component, this ticket's own
// second acceptance criterion, verbatim ("using the existing diff data, no new diff
// computation in the UI layer"). There is no write/edit action anywhere here (this
// ticket's own third criterion) — same as every other view in this app.
export function GraphView({ version }: { version: AssessResponse }) {
  const delta = version.assurance_delta ?? [];
  const hasSVG = typeof version.graph === "string" && version.graph.includes("<svg");

  return (
    <div>
      <h2 style={{ fontSize: 14, margin: "0 0 8px" }}>V{version.version_number} — architecture graph</h2>
      {hasSVG ? (
        <div
          style={{ border: "1px solid #e2e8f0", overflow: "auto", maxHeight: 520, background: "#fff" }}
          // eslint-disable-next-line react/no-danger -- see file header: trusted, server-generated SVG, rendered verbatim per this ticket's own "without modification" criterion.
          dangerouslySetInnerHTML={{ __html: version.graph ?? "" }}
        />
      ) : (
        <p style={{ color: "#b91c1c", fontSize: 12 }}>{version.graph ?? "no graph available"}</p>
      )}

      <h3 style={{ fontSize: 13, margin: "16px 0 6px" }}>
        Diff vs. V{version.version_number - 1}
      </h3>
      {version.version_number <= 1 ? (
        <p style={{ color: "#64748b", fontSize: 12 }}>No prior version to diff against.</p>
      ) : delta.length === 0 ? (
        <p style={{ color: "#64748b", fontSize: 12 }}>No changes.</p>
      ) : (
        <ul style={{ fontSize: 12, paddingLeft: 18 }}>
          {delta.map((d) => (
            <li key={d.finding_id}>
              <code>{d.finding_id}</code>: <strong>{d.kind}</strong>
              {d.old_status && d.new_status ? ` (${d.old_status} → ${d.new_status})` : ""}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
