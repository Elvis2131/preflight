import { useEffect, useState, useCallback } from "react";
import type { Edge, Node } from "@xyflow/react";
import type { CanvasEdgeData, CanvasNodeData } from "./types";
import {
  listScenarios,
  saveScenario,
  deleteScenario,
  type Fault,
  type SavedScenario,
  type ScenarioResult,
} from "./api";
import { FAULT_KINDS, targetsFor, registeredTargets, buildFault, describeFault, ruleLabel, type FaultKind } from "./faultBuilder";

// FailureLab (PC-131) is the Failure Lab mode's scenario panel: pick faults from the
// architecture, build a scenario of one or more, run it, save it by name, and re-run
// every saved scenario against the CURRENT design. It names faults and displays what the
// server returned — which journeys fail or degrade, the verdict, what is severed — and
// computes none of it. Saved scenarios are definitions only; every result shown here was
// just computed by the server against the current design, never stored or replayed.

const box: React.CSSProperties = { border: "1px solid var(--line)", borderRadius: 6, padding: 8, marginBottom: 10 };
const small: React.CSSProperties = { fontSize: 11, color: "var(--muted)" };

export function FailureLab({
  sessionID,
  nodes,
  edges = [],
  regions,
  busy,
  onRun,
  onRerunAll,
  results,
  resultsVersion,
}: {
  sessionID: string;
  nodes: Node<CanvasNodeData>[];
  edges?: Edge<CanvasEdgeData>[];
  regions: string[];
  busy: boolean;
  onRun: (faults: Fault[]) => void;
  onRerunAll: () => void;
  results: ScenarioResult[] | null;
  resultsVersion: number | null;
}) {
  const [kind, setKind] = useState<FaultKind>("node_loss");
  const [target, setTarget] = useState("");
  const [ruleIndex, setRuleIndex] = useState(0);
  const [deregister, setDeregister] = useState("");
  const [faults, setFaults] = useState<Fault[]>([]);
  const [name, setName] = useState("");
  const [saved, setSaved] = useState<SavedScenario[]>([]);
  const [error, setError] = useState<string | null>(null);

  const targets = targetsFor(kind, nodes, regions, edges);
  const lbTargets = kind === "target_deregistration" ? registeredTargets(target, nodes, edges) : [];
  const targetNode = nodes.find((n) => n.id === target);
  const rules = targetNode?.data.securityGroupRules ?? [];

  const refresh = useCallback(() => {
    listScenarios(sessionID)
      .then(setSaved)
      .catch(() => setSaved([])); // no backend yet / empty session: the list is simply empty
  }, [sessionID]);
  useEffect(refresh, [refresh]);

  // Keep the target valid when the kind (or the design) changes.
  useEffect(() => {
    if (!targets.some((t) => t.value === target)) setTarget(targets[0]?.value ?? "");
    setRuleIndex(0);
  }, [kind, nodes.length, edges.length, regions.join("|")]); // eslint-disable-line react-hooks/exhaustive-deps

  // Keep the picked registered target valid for the chosen load balancer.
  useEffect(() => {
    if (!lbTargets.some((t) => t.value === deregister)) setDeregister(lbTargets[0]?.value ?? "");
  }, [kind, target, nodes.length, edges.length]); // eslint-disable-line react-hooks/exhaustive-deps

  const addFault = () => {
    setError(null);
    if (!target) return;
    try {
      setFaults((fs) => [...fs, buildFault(kind, target, kind === "sg_rule_removal" ? rules[ruleIndex] : undefined, kind === "target_deregistration" ? deregister : undefined)]);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  const save = async () => {
    setError(null);
    try {
      await saveScenario(sessionID, { name: name.trim(), faults });
      setName("");
      refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  return (
    <div className="result-panel" style={{ padding: 16, fontSize: 12 }} data-testid="failure-lab">
      <h3 style={{ fontSize: 13, margin: "0 0 8px" }}>Failure Lab — scenario</h3>

      <div className="lab-card" style={box}>
        <div style={small}>Pick a fault from the architecture, then add it. A scenario can combine several.</div>
        <select value={kind} onChange={(e) => setKind(e.target.value as FaultKind)} data-testid="fault-kind" style={{ width: "100%", margin: "6px 0" }}>
          {FAULT_KINDS.map((k) => (
            <option key={k.id} value={k.id}>
              {k.label}
            </option>
          ))}
        </select>
        {targets.length === 0 ? (
          <div style={{ ...small, color: "var(--warning-ink)" }}>Nothing in this design a "{kind}" fault could apply to.</div>
        ) : (
          <select value={target} onChange={(e) => setTarget(e.target.value)} data-testid="fault-target" style={{ width: "100%", marginBottom: 6 }}>
            {targets.map((t) => (
              <option key={t.value} value={t.value}>
                {t.label}
              </option>
            ))}
          </select>
        )}
        {kind === "sg_rule_removal" && rules.length > 0 && (
          <select value={ruleIndex} onChange={(e) => setRuleIndex(Number(e.target.value))} data-testid="fault-rule" style={{ width: "100%", marginBottom: 6 }}>
            {rules.map((r, i) => (
              <option key={i} value={i}>
                {ruleLabel(r)}
              </option>
            ))}
          </select>
        )}
        {kind === "target_deregistration" && lbTargets.length > 0 && (
          <select value={deregister} onChange={(e) => setDeregister(e.target.value)} data-testid="fault-deregister-target" style={{ width: "100%", marginBottom: 6 }}>
            {lbTargets.map((t) => (
              <option key={t.value} value={t.value}>
                {t.label}
              </option>
            ))}
          </select>
        )}
        <button onClick={addFault} disabled={!target || (kind === "target_deregistration" && !deregister)} data-testid="add-fault">
          + add fault
        </button>
      </div>

      <div className="lab-card" style={box}>
        <strong>Scenario ({faults.length} fault{faults.length === 1 ? "" : "s"})</strong>
        {faults.length === 0 && <div style={small}>No faults yet.</div>}
        <ul style={{ paddingLeft: 16, margin: "4px 0" }} data-testid="scenario-faults">
          {faults.map((f, i) => (
            <li key={i}>
              {describeFault(f)}{" "}
              <button style={{ fontSize: 10 }} onClick={() => setFaults((fs) => fs.filter((_, j) => j !== i))} aria-label="remove fault">
                ×
              </button>
            </li>
          ))}
        </ul>
        <button onClick={() => onRun(faults)} disabled={busy || faults.length === 0} data-testid="run-scenario" style={{ background: "var(--danger-soft)", border: "1px solid var(--danger-ink)" }}>
          {busy ? "Running…" : "Run scenario"}
        </button>
        <div style={{ display: "flex", gap: 4, marginTop: 6 }}>
          <input value={name} onChange={(e) => setName(e.target.value)} placeholder="scenario name" data-testid="scenario-name" style={{ flex: 1, fontSize: 12 }} />
          <button onClick={save} disabled={!name.trim() || faults.length === 0} data-testid="save-scenario">
            Save
          </button>
        </div>
        {error && <div style={{ color: "var(--danger-ink)", fontSize: 11, marginTop: 4 }}>{error}</div>}
      </div>

      <div className="lab-card" style={box}>
        <strong>Saved scenarios ({saved.length})</strong>
        {saved.length === 0 && <div style={small}>None saved for this session.</div>}
        <ul style={{ paddingLeft: 0, listStyle: "none", margin: "4px 0" }} data-testid="saved-scenarios">
          {saved.map((sc) => (
            <li key={sc.name} style={{ borderTop: "1px solid var(--surface-soft)", padding: "4px 0" }}>
              <div style={{ fontWeight: 600 }}>{sc.name}</div>
              <div style={small}>{sc.faults.map(describeFault).join(" + ")}</div>
              <button style={{ fontSize: 10 }} onClick={() => setFaults(sc.faults)}>
                load
              </button>{" "}
              <button style={{ fontSize: 10 }} disabled={busy} onClick={() => onRun(sc.faults)}>
                run
              </button>{" "}
              <button
                style={{ fontSize: 10 }}
                onClick={async () => {
                  await deleteScenario(sessionID, sc.name);
                  refresh();
                }}
              >
                delete
              </button>
            </li>
          ))}
        </ul>
        <button onClick={onRerunAll} disabled={busy || saved.length === 0} data-testid="rerun-all">
          Re-run all against the current design
        </button>
      </div>

      {results && (
        <div className="lab-card result-card" style={box} data-testid="scenario-results">
          <strong>Results — current design (v{resultsVersion})</strong>
          <div style={small}>Computed by the server just now, from the saved definitions.</div>
          <ul style={{ listStyle: "none", padding: 0, margin: "4px 0 0" }}>
            {results.map((r) => (
              <li key={r.name} style={{ borderTop: "1px solid var(--line)", padding: "6px 0", overflowWrap: "anywhere" }}>
                <div style={{ fontWeight: 600 }}>{r.name}</div>
                <div style={{ color: r.verdict.state === "assessed" ? "var(--ink)" : "var(--subtle)" }}>
                  {r.verdict.state === "assessed" ? `verdict: ${String(r.verdict.value)}` : `not assessable: ${r.verdict.reason}`}
                </div>
                {r.failed_journeys.length > 0 && <div style={{ color: "var(--danger-ink)" }}>failed journeys: {r.failed_journeys.join(", ")}</div>}
                {r.degraded_journeys.length > 0 && <div style={{ color: "var(--warning-ink)" }}>degraded journeys: {r.degraded_journeys.join(", ")}</div>}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
