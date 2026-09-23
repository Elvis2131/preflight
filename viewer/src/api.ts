// api.ts is the ONLY place this app talks to assessd (P1) over HTTP. This is PC-90's
// own read-only viewer (PRD §6) — every type here mirrors a real Go response shape by
// field name, cross-checked against server/history.go's AssessResponse and
// core/scorecard.go's Scorecard/ScorecardEntry/DeltaEntry, not guessed.

export const ASSESSD_BASE_URL = "http://localhost:8080";

export interface Provenance {
  kind: string;
  source: string;
  reason?: string;
}

export interface ScorecardEntry {
  finding_id: string;
  status: string;
  provenance: Provenance;
}

export interface Scorecard {
  version_number: number;
  entries: ScorecardEntry[];
}

// DeltaKind mirrors core.DeltaKind's real 6 values (core/internal/analyse/delta.go) —
// never treated as an ordered/numeric scale anywhere in this app.
export type DeltaKind =
  | "improvement"
  | "regression"
  | "new_risk"
  | "unchanged"
  | "not_assessable"
  | "resolved_risk";

export interface DeltaEntry {
  finding_id: string;
  kind: DeltaKind;
  old_status?: string;
  new_status?: string;
  reason?: string;
  provenance: Provenance;
}

export interface AssessResponse {
  session_id: string;
  version_number: number;
  findings: unknown[];
  scorecard: Scorecard;
  assurance_delta?: DeltaEntry[];
  graph: string | null;
  degraded: boolean;
  degraded_reason?: string;
  compute_duration_ms: number;
}

export class APIError extends Error {
  code: string;
  status: number;
  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = "APIError";
    this.status = status;
    this.code = code;
  }
}

async function getJSON<T>(url: string): Promise<T> {
  const res = await fetch(url);
  if (!res.ok) {
    const text = await res.text();
    try {
      const parsed = JSON.parse(text) as { error_code?: string; message?: string };
      if (parsed.error_code && parsed.message) {
        throw new APIError(res.status, parsed.error_code, parsed.message);
      }
    } catch (parseErr) {
      if (parseErr instanceof APIError) throw parseErr;
    }
    throw new Error(`${res.status} ${res.statusText}: ${text}`);
  }
  return res.json() as Promise<T>;
}

// listVersions calls GET /sessions/{id}/versions (PC-92 groundwork) — every version
// 1..latest for a session, each already carrying its own real AssuranceDelta computed
// server-side. This app never recomputes a delta, a status, or a trend of any kind —
// it only renders exactly what this one call returns.
export function listVersions(
  sessionID: string,
  baseURL: string = ASSESSD_BASE_URL,
): Promise<AssessResponse[]> {
  return getJSON(`${baseURL}/sessions/${encodeURIComponent(sessionID)}/versions`);
}
