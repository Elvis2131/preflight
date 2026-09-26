import { useState } from "react";
import type { Requirement, RequirementPriority, Workload, DeclaredJourney } from "./workloadTypes";
import { WORKLOAD_SCHEMA_VERSION } from "./workloadTypes";

// WorkloadForm is PC-87's own scope: "a UI over the existing workload.yaml schema —
// not a new or looser schema." Every field below maps 1:1 onto core.Workload; there is
// no field here that doesn't exist in workload.schema.json, and no relaxation of what
// that schema requires.

interface CapacityRow {
  key: string;
  // Stored as text, deliberately: an EMPTY string here means "not declared" (the
  // form's own version of core/workload.go's "no declared capacity ->
  // capacity_unknown" rule). Coercing this to 0 at any point before submit would
  // silently turn "left blank" into a fabricated zero value — exactly what PC-87's own
  // Conversation warns against ("must not make it easier to accidentally skip this
  // than a YAML file already makes it").
  valueText: string;
}

interface RequirementRow {
  id: string;
  value: string;
  priority: RequirementPriority;
  rankText: string; // only meaningful/required when priority === "preference"
}

function emptyRequirementRow(): RequirementRow {
  return { id: "", value: "", priority: "hard", rankText: "" };
}

// IMPLEMENTED_FRAMEWORKS is PC-110's own explicit list — a framework selectable here
// must have a real, implemented control catalog (core/compliance_*.go), never an
// aspirational one. Identifiers are core.ComplianceFramework's own real string
// values (core/compliance_catalog.go), not invented for this form, so a value
// written here is directly usable by whatever future report/compliance consumer
// filters by compliance_profiles. "comingSoon" entries are shown, visibly
// non-selectable — the Card's own explicit instruction — rather than omitted, so an
// architect knows a framework exists on the roadmap without being able to pick it
// before this engine can actually assess it.
interface FrameworkOption {
  id: string;
  label: string;
  comingSoon?: string; // reason shown when disabled; absent means selectable
}
export const COMPLIANCE_FRAMEWORK_OPTIONS: FrameworkOption[] = [
  { id: "cis_aws", label: "CIS AWS Foundations Benchmark" },
  { id: "pci_dss_4", label: "PCI DSS v4.0" },
  { id: "soc2", label: "SOC 2" },
  { id: "hipaa", label: "HIPAA", comingSoon: "no control catalog implemented yet" },
];

// JourneyRow (PC-127) — pathText is comma-separated IR node IDs (or "internet" as
// the first hop). peakRPSText/steadyRPSText are text, same "blank means not
// declared, never coerced to 0" discipline as CapacityRow.valueText above.
interface JourneyRow {
  id: string;
  name: string;
  pathText: string;
  protocol: string;
  port: string;
  criticality: string;
  peakRPSText: string;
  steadyRPSText: string;
}

function emptyJourneyRow(): JourneyRow {
  return { id: "", name: "", pathText: "", protocol: "tcp", port: "", criticality: "", peakRPSText: "", steadyRPSText: "" };
}

export interface WorkloadFormValue {
  name: string;
  criticality: string;
  dataClassification: string;
  regionsText: string; // comma-separated
  complianceProfiles: string[]; // PC-110: only IMPLEMENTED_FRAMEWORKS ids, never free text
  capacityRows: CapacityRow[];
  requirementRows: RequirementRow[];
  journeyRows: JourneyRow[];
}

export function emptyWorkloadFormValue(): WorkloadFormValue {
  return {
    name: "",
    criticality: "",
    dataClassification: "",
    regionsText: "",
    complianceProfiles: [],
    capacityRows: [{ key: "app_node_rps", valueText: "" }],
    requirementRows: [],
    journeyRows: [],
  };
}

function splitCommaList(text: string): string[] {
  return text
    .split(",")
    .map((s) => s.trim())
    .filter((s) => s.length > 0);
}

// buildWorkload turns the form's own free-text working state into a real
// core.Workload-shaped object — the ONE function permitted to do this, same
// discipline serialize.ts already established for CanvasDocument. A capacity row with
// a blank valueText is DROPPED from the output map entirely, never sent as 0; a
// requirement row with priority "hard" never carries a rank key at all, matching
// core.Requirement's own excluded_if tag rather than relying on the server to reject
// a value this form could simply not send.
export function buildWorkload(v: WorkloadFormValue): Workload {
  const capacity: Record<string, number> = {};
  for (const row of v.capacityRows) {
    const key = row.key.trim();
    if (!key || row.valueText.trim() === "") continue;
    const num = Number(row.valueText);
    if (Number.isFinite(num)) capacity[key] = num;
  }

  const requirements: Requirement[] = v.requirementRows
    .filter((r) => r.id.trim() !== "")
    .map((r) => {
      const req: Requirement = { id: r.id.trim(), value: r.value, priority: r.priority };
      if (r.priority === "preference") {
        const rank = Number(r.rankText);
        if (Number.isFinite(rank)) req.rank = rank;
      }
      return req;
    });

  // journeys (PC-127): a row missing id/name/a real 2+ node path/protocol/port/
  // criticality is dropped — same "an empty row is not a real declaration" rule
  // requirementRows already applies. peak_rps/steady_rps stay omitted (never 0) when
  // their own text field is blank.
  const journeys: DeclaredJourney[] = v.journeyRows
    .map((r) => {
      const path = splitCommaList(r.pathText);
      const port = Number(r.port);
      if (!r.id.trim() || !r.name.trim() || path.length < 2 || !r.protocol.trim() || !Number.isFinite(port) || !r.criticality.trim()) {
        return null;
      }
      const j: DeclaredJourney = {
        id: r.id.trim(), name: r.name.trim(), path, protocol: r.protocol.trim(), port, criticality: r.criticality.trim(),
      };
      const peak = Number(r.peakRPSText);
      if (r.peakRPSText.trim() !== "" && Number.isFinite(peak)) j.peak_rps = peak;
      const steady = Number(r.steadyRPSText);
      if (r.steadyRPSText.trim() !== "" && Number.isFinite(steady)) j.steady_rps = steady;
      return j;
    })
    .filter((j): j is DeclaredJourney => j !== null);

  return {
    schema_version: WORKLOAD_SCHEMA_VERSION,
    name: v.name.trim(),
    criticality: v.criticality.trim(),
    data_classification: v.dataClassification.trim(),
    regions: splitCommaList(v.regionsText),
    compliance_profiles: v.complianceProfiles,
    requirements,
    capacity: Object.keys(capacity).length > 0 ? capacity : undefined,
    journeys: journeys.length > 0 ? journeys : undefined,
  };
}

const inputStyle: React.CSSProperties = { fontSize: 12, width: "100%", boxSizing: "border-box" };
const rowStyle: React.CSSProperties = { display: "flex", gap: 6, alignItems: "center", marginBottom: 4 };
const labelStyle: React.CSSProperties = { fontSize: 11, color: "#475569", display: "block", marginBottom: 2 };

export function WorkloadForm({
  value,
  onChange,
  pickingPathForRow = null,
  onStartPickPath,
  onStopPickPath,
}: {
  value: WorkloadFormValue;
  onChange: (v: WorkloadFormValue) => void;
  // pickingPathForRow/onStartPickPath/onStopPickPath (PC-124's own stated
  // acceptance criterion: "journey paths can be picked on the canvas by clicking
  // components in order") — App.tsx owns node-click handling, so path-picking mode
  // lives there; this form only starts/stops it and shows which row (if any) is
  // currently being picked. Optional so existing callers/tests that don't need
  // canvas picking (e.g. buildWorkload unit tests) need no changes.
  pickingPathForRow?: number | null;
  onStartPickPath?: (rowIndex: number) => void;
  onStopPickPath?: () => void;
}) {
  const [showRequirementHelp, setShowRequirementHelp] = useState(false);

  const update = (patch: Partial<WorkloadFormValue>) => onChange({ ...value, ...patch });

  const hardReqs = value.requirementRows
    .map((r, i) => ({ r, i }))
    .filter(({ r }) => r.priority === "hard");
  const preferenceReqs = value.requirementRows
    .map((r, i) => ({ r, i }))
    .filter(({ r }) => r.priority === "preference");

  function updateRow(i: number, patch: Partial<RequirementRow>) {
    const rows = value.requirementRows.slice();
    rows[i] = { ...rows[i], ...patch };
    update({ requirementRows: rows });
  }
  function removeRow(i: number) {
    update({ requirementRows: value.requirementRows.filter((_, idx) => idx !== i) });
  }

  return (
    <div style={{ padding: 12, fontSize: 12 }}>
      <h3 style={{ fontSize: 13, margin: "0 0 8px" }}>Workload (NFR form)</h3>
      <p style={{ fontSize: 11, color: "#64748b", margin: "0 0 10px" }}>
        Same schema as workload.yaml (PRD §4) — no separate or looser shape. A capacity
        field left blank is <code>capacity_unknown</code>, exactly as an omitted key in
        a hand-written YAML file would be.
      </p>

      <label style={labelStyle}>name</label>
      <input style={inputStyle} value={value.name} onChange={(e) => update({ name: e.target.value })} />

      <label style={{ ...labelStyle, marginTop: 8 }}>criticality (e.g. tier1)</label>
      <input
        style={inputStyle}
        value={value.criticality}
        onChange={(e) => update({ criticality: e.target.value })}
      />

      <label style={{ ...labelStyle, marginTop: 8 }}>data_classification</label>
      <input
        style={inputStyle}
        value={value.dataClassification}
        onChange={(e) => update({ dataClassification: e.target.value })}
      />

      <label style={{ ...labelStyle, marginTop: 8 }}>regions (comma-separated)</label>
      <input
        style={inputStyle}
        value={value.regionsText}
        onChange={(e) => update({ regionsText: e.target.value })}
        placeholder="eu-west-1"
      />

      <label style={{ ...labelStyle, marginTop: 8 }}>compliance_profiles</label>
      <p style={{ fontSize: 10, color: "#94a3b8", margin: "0 0 4px" }}>
        Only frameworks with an implemented control catalog are selectable — never
        offer one this engine cannot actually assess.
      </p>
      {COMPLIANCE_FRAMEWORK_OPTIONS.map((fw) => (
        <label
          key={fw.id}
          style={{
            display: "flex",
            alignItems: "center",
            gap: 6,
            fontSize: 11,
            marginBottom: 2,
            color: fw.comingSoon ? "#94a3b8" : "#0f172a",
            cursor: fw.comingSoon ? "not-allowed" : "pointer",
          }}
        >
          <input
            type="checkbox"
            disabled={!!fw.comingSoon}
            checked={value.complianceProfiles.includes(fw.id)}
            onChange={(e) =>
              update({
                complianceProfiles: e.target.checked
                  ? [...value.complianceProfiles, fw.id]
                  : value.complianceProfiles.filter((id) => id !== fw.id),
              })
            }
          />
          {fw.label}
          {fw.comingSoon && <span style={{ fontStyle: "italic" }}> — coming soon ({fw.comingSoon})</span>}
        </label>
      ))}

      <h4 style={{ fontSize: 12, margin: "14px 0 4px" }}>Capacity</h4>
      <p style={{ fontSize: 10, color: "#94a3b8", margin: "0 0 6px" }}>
        Leave the value blank to leave this capacity <strong>undeclared</strong> — it
        will NOT be sent as zero.
      </p>
      {value.capacityRows.map((row, i) => (
        <div key={i} style={rowStyle}>
          <input
            style={{ ...inputStyle, flex: 1 }}
            value={row.key}
            placeholder="app_node_rps"
            onChange={(e) => {
              const rows = value.capacityRows.slice();
              rows[i] = { ...rows[i], key: e.target.value };
              update({ capacityRows: rows });
            }}
          />
          <input
            style={{ ...inputStyle, width: 80 }}
            value={row.valueText}
            placeholder="(blank = unknown)"
            onChange={(e) => {
              const rows = value.capacityRows.slice();
              rows[i] = { ...rows[i], valueText: e.target.value };
              update({ capacityRows: rows });
            }}
          />
          <button
            onClick={() => update({ capacityRows: value.capacityRows.filter((_, idx) => idx !== i) })}
          >
            ×
          </button>
        </div>
      ))}
      <button onClick={() => update({ capacityRows: [...value.capacityRows, { key: "", valueText: "" }] })}>
        + capacity field
      </button>

      <h4 style={{ fontSize: 12, margin: "14px 0 4px" }}>
        Requirements{" "}
        <button style={{ fontSize: 10 }} onClick={() => setShowRequirementHelp((v) => !v)}>
          ?
        </button>
      </h4>
      {showRequirementHelp && (
        <p style={{ fontSize: 10, color: "#64748b", margin: "0 0 6px" }}>
          hard = a constraint whose violation is a FAILING finding. preference = a
          ranked goal that trades against other preferences rather than failing
          outright — it needs a rank so preferences can be ordered against each other.
        </p>
      )}

      <div style={{ display: "flex", gap: 10 }}>
        <div style={{ flex: 1, border: "1px solid #fca5a5", borderRadius: 4, padding: 6, background: "#fef2f2" }}>
          <div style={{ fontSize: 11, fontWeight: 700, color: "#b91c1c", marginBottom: 4 }}>
            HARD (constraint)
          </div>
          {hardReqs.map(({ r, i }) => (
            <RequirementRowEditor key={i} row={r} onChange={(patch) => updateRow(i, patch)} onRemove={() => removeRow(i)} />
          ))}
          <button onClick={() => update({ requirementRows: [...value.requirementRows, { ...emptyRequirementRow(), priority: "hard" }] })}>
            + hard requirement
          </button>
        </div>
        <div style={{ flex: 1, border: "1px solid #93c5fd", borderRadius: 4, padding: 6, background: "#eff6ff" }}>
          <div style={{ fontSize: 11, fontWeight: 700, color: "#1d4ed8", marginBottom: 4 }}>
            PREFERENCE (ranked goal)
          </div>
          {preferenceReqs.map(({ r, i }) => (
            <RequirementRowEditor key={i} row={r} onChange={(patch) => updateRow(i, patch)} onRemove={() => removeRow(i)} />
          ))}
          <button
            onClick={() =>
              update({ requirementRows: [...value.requirementRows, { ...emptyRequirementRow(), priority: "preference" }] })
            }
          >
            + preference requirement
          </button>
        </div>
      </div>

      <h4 style={{ fontSize: 12, margin: "14px 0 4px" }}>Journeys (PC-124/127)</h4>
      <p style={{ fontSize: 10, color: "#94a3b8", margin: "0 0 6px" }}>
        Path is a comma-separated list of canvas node IDs (or "internet" as the first
        hop). peak_rps/steady_rps left blank stay undeclared — traffic/load results
        report not_assessable for that journey, never a guessed number.
      </p>
      {value.journeyRows.map((row, i) => (
        <div key={i} style={{ marginBottom: 6, paddingBottom: 6, borderBottom: "1px dashed #cbd5e1" }}>
          <div style={rowStyle}>
            <input
              style={{ ...inputStyle, flex: 1 }}
              placeholder="id"
              value={row.id}
              onChange={(e) => {
                const rows = value.journeyRows.slice();
                rows[i] = { ...rows[i], id: e.target.value };
                update({ journeyRows: rows });
              }}
            />
            <input
              style={{ ...inputStyle, flex: 1 }}
              placeholder="name"
              value={row.name}
              onChange={(e) => {
                const rows = value.journeyRows.slice();
                rows[i] = { ...rows[i], name: e.target.value };
                update({ journeyRows: rows });
              }}
            />
            <button onClick={() => update({ journeyRows: value.journeyRows.filter((_, idx) => idx !== i) })}>×</button>
          </div>
          <div style={rowStyle}>
            <input
              style={{ ...inputStyle, flex: 1, marginBottom: 4 }}
              placeholder="path (e.g. internet, lb-1, db-1)"
              value={row.pathText}
              onChange={(e) => {
                const rows = value.journeyRows.slice();
                rows[i] = { ...rows[i], pathText: e.target.value };
                update({ journeyRows: rows });
              }}
            />
            <button
              title='Prepend "internet" as the first hop — core.JourneyInternetSentinel, for an internet-originated journey'
              onClick={() => {
                const rows = value.journeyRows.slice();
                const hops = row.pathText.split(",").map((h) => h.trim()).filter((h) => h !== "");
                if (hops[0] !== "internet") hops.unshift("internet");
                rows[i] = { ...rows[i], pathText: hops.join(", ") };
                update({ journeyRows: rows });
              }}
              style={{ fontSize: 11, whiteSpace: "nowrap" }}
            >
              + internet start
            </button>
            {onStartPickPath &&
              onStopPickPath &&
              (pickingPathForRow === i ? (
                <button
                  onClick={onStopPickPath}
                  style={{ fontSize: 11, background: "#dcfce7", border: "1px solid #16a34a", whiteSpace: "nowrap" }}
                >
                  Done picking
                </button>
              ) : (
                <button
                  onClick={() => onStartPickPath(i)}
                  disabled={pickingPathForRow !== null}
                  style={{ fontSize: 11, whiteSpace: "nowrap" }}
                >
                  Pick path on canvas
                </button>
              ))}
          </div>
          {pickingPathForRow === i && (
            <p style={{ fontSize: 10, color: "#7c3aed", margin: "0 0 4px" }}>
              Click canvas nodes in order to append them to this path. Click "Done
              picking" when finished.
            </p>
          )}
          <div style={rowStyle}>
            <input
              style={{ ...inputStyle, width: 70 }}
              placeholder="protocol"
              value={row.protocol}
              onChange={(e) => {
                const rows = value.journeyRows.slice();
                rows[i] = { ...rows[i], protocol: e.target.value };
                update({ journeyRows: rows });
              }}
            />
            <input
              style={{ ...inputStyle, width: 60 }}
              placeholder="port"
              value={row.port}
              onChange={(e) => {
                const rows = value.journeyRows.slice();
                rows[i] = { ...rows[i], port: e.target.value };
                update({ journeyRows: rows });
              }}
            />
            <input
              style={{ ...inputStyle, width: 70 }}
              placeholder="criticality"
              value={row.criticality}
              onChange={(e) => {
                const rows = value.journeyRows.slice();
                rows[i] = { ...rows[i], criticality: e.target.value };
                update({ journeyRows: rows });
              }}
            />
            <input
              style={{ ...inputStyle, width: 80 }}
              placeholder="peak_rps"
              value={row.peakRPSText}
              onChange={(e) => {
                const rows = value.journeyRows.slice();
                rows[i] = { ...rows[i], peakRPSText: e.target.value };
                update({ journeyRows: rows });
              }}
            />
            <input
              style={{ ...inputStyle, width: 80 }}
              placeholder="steady_rps"
              value={row.steadyRPSText}
              onChange={(e) => {
                const rows = value.journeyRows.slice();
                rows[i] = { ...rows[i], steadyRPSText: e.target.value };
                update({ journeyRows: rows });
              }}
            />
          </div>
        </div>
      ))}
      <button onClick={() => update({ journeyRows: [...value.journeyRows, emptyJourneyRow()] })}>+ journey</button>
    </div>
  );
}

function RequirementRowEditor({
  row,
  onChange,
  onRemove,
}: {
  row: RequirementRow;
  onChange: (patch: Partial<RequirementRow>) => void;
  onRemove: () => void;
}) {
  return (
    <div style={{ marginBottom: 6, paddingBottom: 6, borderBottom: "1px dashed #cbd5e1" }}>
      <div style={rowStyle}>
        <input
          style={{ ...inputStyle, flex: 1 }}
          placeholder="id"
          value={row.id}
          onChange={(e) => onChange({ id: e.target.value })}
        />
        <button onClick={onRemove}>×</button>
      </div>
      <input
        style={{ ...inputStyle, marginBottom: 4 }}
        placeholder="value"
        value={row.value}
        onChange={(e) => onChange({ value: e.target.value })}
      />
      {row.priority === "preference" && (
        <input
          style={inputStyle}
          type="number"
          placeholder="rank (required for preference)"
          value={row.rankText}
          onChange={(e) => onChange({ rankText: e.target.value })}
        />
      )}
    </div>
  );
}
