// narratives.ts (PC-154) is the Analyze mode's client for LLM-written annotations. It displays
// what the server's annotation stream carries and decides nothing: a narrative is prose beside a
// finding, never a verdict (I3), and a missing, down or rate-limited narrative layer is a
// `degraded` state, never an error — the findings are complete either way.

import { ASSESSD_BASE_URL } from "../api";
import type { Provenance } from "./api";

export interface Narrative {
  finding_id: string;
  narrative: string;
  cited_evidence: string[];
  provenance: Provenance;
}

export interface NarrativeRejection {
  finding_id: string;
  reason: string;
  kind: string;
}

// StoredNarratives mirrors core.LLMAnnotationSet (GET .../annotations?format=json).
export interface StoredNarratives {
  status: "complete" | "degraded";
  reason?: string;
  model?: string;
  annotations: Narrative[];
  rejected: NarrativeRejection[];
  notice: string;
}

export const LLM_NOTICE =
  "Written by a language model from the findings; it explains them and decides nothing. The findings, their statuses and every number are the engine's, not the model's.";

export type NarrativePhase = "idle" | "streaming" | "complete" | "degraded";

export interface NarrativeState {
  phase: NarrativePhase;
  annotations: Narrative[];
  rejected: NarrativeRejection[];
  degradedReason?: string;
  model?: string;
}

export const initialNarrativeState: NarrativeState = { phase: "idle", annotations: [], rejected: [] };

export type NarrativeEvent =
  | { type: "reset" }
  | { type: "load"; state: NarrativeState }
  | { type: "start" }
  | { type: "annotation"; data: Narrative }
  | { type: "rejected"; data: NarrativeRejection }
  | { type: "degraded"; reason: string }
  | { type: "done"; status: string }
  | { type: "interrupted" };

// narrativeReducer folds the server's stream events into display state. Pure, so the behaviour is
// unit-tested without a browser.
export function narrativeReducer(state: NarrativeState, ev: NarrativeEvent): NarrativeState {
  switch (ev.type) {
    case "reset":
      return initialNarrativeState;
    case "load":
      return ev.state;
    case "start":
      return { ...initialNarrativeState, phase: "streaming" };
    case "annotation":
      return { ...state, annotations: [...state.annotations, ev.data] };
    case "rejected":
      return { ...state, rejected: [...state.rejected, ev.data] };
    case "degraded":
      return { ...state, degradedReason: ev.reason };
    case "done":
      // `done` carries the final status; a degraded reason seen earlier is kept.
      return { ...state, phase: ev.status === "complete" && !state.degradedReason ? "complete" : "degraded" };
    case "interrupted":
      return state.phase === "streaming"
        ? { ...state, phase: "degraded", degradedReason: state.degradedReason ?? "the connection to the server was interrupted before the narratives finished" }
        : state;
  }
}

export function fromStored(set: StoredNarratives): NarrativeState {
  return {
    phase: set.status === "complete" ? "complete" : "degraded",
    annotations: set.annotations,
    rejected: set.rejected,
    degradedReason: set.status === "degraded" ? set.reason : undefined,
    model: set.model,
  };
}

// getStoredNarratives returns the stored set, or null when none is stored (404).
export async function getStoredNarratives(sessionID: string, version: number, baseURL: string = ASSESSD_BASE_URL): Promise<StoredNarratives | null> {
  const res = await fetch(`${baseURL}/sessions/${encodeURIComponent(sessionID)}/versions/${version}/annotations?format=json`);
  if (res.status === 404) return null;
  if (!res.ok) throw new Error(`annotations: HTTP ${res.status}`);
  return (await res.json()) as StoredNarratives;
}

// streamNarratives opens the annotation stream and feeds each event to onEvent. Returns a close
// function. EventSource would auto-reconnect (and re-trigger work), so it is closed on `done` and
// on any error.
export function streamNarratives(sessionID: string, version: number, onEvent: (e: NarrativeEvent) => void, baseURL: string = ASSESSD_BASE_URL): () => void {
  const es = new EventSource(`${baseURL}/sessions/${encodeURIComponent(sessionID)}/versions/${version}/annotations`);
  let finished = false;
  const parse = (m: MessageEvent) => JSON.parse(m.data);
  onEvent({ type: "start" });
  es.addEventListener("annotation", (m) => onEvent({ type: "annotation", data: parse(m as MessageEvent) }));
  es.addEventListener("rejected", (m) => onEvent({ type: "rejected", data: parse(m as MessageEvent) }));
  es.addEventListener("degraded", (m) => onEvent({ type: "degraded", reason: parse(m as MessageEvent).reason }));
  es.addEventListener("done", (m) => {
    finished = true;
    onEvent({ type: "done", status: parse(m as MessageEvent).status });
    es.close();
  });
  es.onerror = () => {
    if (!finished) onEvent({ type: "interrupted" });
    es.close();
  };
  return () => es.close();
}

// The report endpoint attaches stored narratives as their own optional `narratives` section
// (report.schema.json 1.8.0). Typed here, beside the other narrative types, so it does not need
// to touch the shared Report interface.
export type WithNarratives<T> = T & { narratives?: StoredNarratives };
