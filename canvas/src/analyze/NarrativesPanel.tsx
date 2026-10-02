import { useEffect, useReducer, useRef, useState } from "react";
import { fromStored, getStoredNarratives, initialNarrativeState, LLM_NOTICE, narrativeReducer, streamNarratives, type NarrativeState } from "./narratives";

// NarrativesPanel (PC-154) shows the LLM-written explanations of ONE version's findings, visibly
// separate from them and labelled as model-written. Stored narratives show automatically;
// generating new ones is an explicit click because a run costs minutes and tokens. It computes
// nothing: a degraded narrative layer is a banner, never an error, and the findings above are
// complete either way.

const box: React.CSSProperties = { border: "1px dashed #7c3aed", borderRadius: 6, padding: 12, background: "#faf5ff", marginTop: 24 };
const badge: React.CSSProperties = { fontSize: 10, fontWeight: 700, color: "#6d28d9", border: "1px solid #7c3aed", borderRadius: 3, padding: "1px 5px", marginRight: 6 };

export function NarrativesPanel({ sessionID, version }: { sessionID: string; version: number }) {
  const [state, dispatch] = useReducer(narrativeReducer, initialNarrativeState);
  const [loaded, setLoaded] = useState(false);
  const closeRef = useRef<null | (() => void)>(null);

  // Stored narratives for this version show without any action; a different version starts clean.
  useEffect(() => {
    let cancelled = false;
    closeRef.current?.();
    dispatch({ type: "reset" });
    setLoaded(false);
    getStoredNarratives(sessionID, version)
      .then((set) => {
        if (!cancelled && set) dispatch({ type: "load", state: fromStored(set) });
      })
      .catch(() => undefined) // not being able to read stored narratives is not an error state
      .finally(() => !cancelled && setLoaded(true));
    return () => {
      cancelled = true;
      closeRef.current?.();
    };
  }, [sessionID, version]);

  const generate = () => {
    closeRef.current?.();
    closeRef.current = streamNarratives(sessionID, version, dispatch);
  };

  return <PanelBody state={state} loaded={loaded} onGenerate={generate} version={version} />;
}

function PanelBody({ state, loaded, onGenerate, version }: { state: NarrativeState; loaded: boolean; onGenerate: () => void; version: number }) {
  const has = state.annotations.length > 0;
  return (
    <section style={box} data-testid="narratives-panel" aria-label="LLM-written narratives">
      <h2 style={{ fontSize: 14, margin: "0 0 4px" }}>
        <span style={badge}>LLM-WRITTEN</span>Narratives for V{version}
      </h2>
      <p style={{ fontSize: 11, color: "#5b21b6", margin: "0 0 10px" }} data-testid="narratives-notice">
        {LLM_NOTICE}
      </p>

      {state.phase === "degraded" && (
        <div style={{ padding: 8, background: "#fff7ed", border: "1px solid #fdba74", fontSize: 12, marginBottom: 8 }} data-testid="narratives-degraded">
          Narratives {has ? "are incomplete" : "are unavailable"}: {state.degradedReason ?? "the narrative layer could not contribute"}. The findings and the timeline above are unaffected.
        </div>
      )}
      {state.phase === "streaming" && (
        <div style={{ fontSize: 12, color: "#5b21b6", marginBottom: 8 }} data-testid="narratives-streaming">
          Writing narratives… {state.annotations.length} so far
        </div>
      )}

      {(state.phase === "idle" || state.phase === "degraded") && loaded && (
        <button onClick={onGenerate} data-testid="generate-narratives" style={{ fontSize: 12, marginBottom: 8 }}>
          {has ? "Try again" : "Generate narratives"}
        </button>
      )}

      {has && (
        <ul style={{ listStyle: "none", padding: 0, margin: 0 }} data-testid="narratives-list">
          {state.annotations.map((a) => (
            <li key={a.finding_id} style={{ padding: "8px 0", borderTop: "1px solid #e9d5ff" }} data-testid="narrative">
              <div style={{ fontSize: 11, fontFamily: "monospace", color: "#334155" }}>{a.finding_id}</div>
              <div style={{ fontSize: 13, margin: "2px 0" }}>{a.narrative}</div>
              <div style={{ fontSize: 10, color: "#64748b" }}>
                cites: {a.cited_evidence.join(", ")} · provenance: {a.provenance.kind}
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
