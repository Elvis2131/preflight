// api.ts is the ONLY place this app talks to assessd (P1) over HTTP — PC-88's own
// scope note: "the canvas never computes a fault result itself." Every type below
// mirrors a real Go response shape by field name, not guessed — cross-checked against
// server/canvas.go's AssessResponse and core/simulate.go's SimulateResponse/
// AssessmentEnvelope before being typed here.
import type { CanvasDocument } from "./types";
import type { Workload } from "./workloadTypes";

export const ASSESSD_BASE_URL = "http://localhost:8080";

export interface AssessmentEnvelope {
  state: "assessed" | "not_assessable";
  value?: unknown;
  reason?: string;
}

export interface AssessResponse {
  session_id: string;
  version_number: number;
  findings: unknown[];
  scorecard: unknown;
  assurance_delta?: unknown[];
  graph: string | null;
  degraded: boolean;
}

export interface Journey {
  entry_point: string;
  target: string;
  survives: boolean;
}

export interface SimulateResponse {
  journeys: Journey[] | null;
  capacity: AssessmentEnvelope;
  // core.Simulate (core/simulate.go) now initializes severed_paths/journeys as a
  // non-nil empty slice, so a real assessd never actually sends null here — fixed at
  // the Go source, not patched only in this file (the null-ability below is
  // boundary-level defense-in-depth against a differently-versioned or malformed
  // server, matching CLAUDE.md's "validate at system boundaries" carve-out, not a
  // guard against a bug this codebase still has).
  severed_paths: string[] | null;
  cascade: string[] | null;
  verdict: AssessmentEnvelope;
}

// APIError (PC-95) mirrors server.APIError's own wire shape ({error_code, message}) —
// `code` is the stable, machine-readable signal a caller can branch on; `message` is
// the same human-readable text a plain error would have carried. Thrown instead of a
// bare Error whenever assessd's response body actually parses as that shape, so a
// catch site can do `err instanceof APIError && err.code === "..."` rather than
// pattern-matching the message string — the entire point PC-95 exists to enable.
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

// describeSimError (PC-95) is the ONE function that turns anything killSelectedNode's
// try/catch might throw into display text — the same "one function, unit-tested,
// negative-controlled" discipline serialize.ts/buildWorkload already established,
// rather than inlining this branch directly in App.tsx where only a live browser
// click could ever exercise it. session_not_found gets a distinct, actionable
// message (this app's own proof that the structured signal is actually read
// somewhere, not just parsed and discarded — PC-95's own acceptance criterion); every
// other error — a plain Error, a different APIError code, a non-Error thrown value —
// falls back to its own message text unchanged, exactly as before PC-95 existed.
export function describeSimError(err: unknown): string {
  if (err instanceof APIError && err.code === "session_not_found") {
    return `This session no longer exists on the server (it may have restarted). Click "Kill selected node" again to start a fresh one. [${err.code}]`;
  }
  return err instanceof Error ? err.message : String(err);
}

// PriceEntry/PricingSnapshot mirror pricing.PriceEntry/pricing.Snapshot's own wire
// shape (server/pricing.go, ADR-006 §4) field-for-field — cross-checked against that
// Go source before typing, not guessed.
export interface PriceEntry {
  service: string;
  region: string;
  sku_attributes: Record<string, string>;
  unit: string;
  price: number;
  currency: string;
}

export interface PricingSnapshot {
  id: string;
  fetched_at: string;
  source: string;
  disclaimer: string;
  entries: PriceEntry[];
  active: boolean;
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

// listPricingSnapshots/getPricingSnapshot (PC-110) back the Design inspector's
// instance-type picker: GET /pricing/snapshots (PC-116) lists what exists; a caller
// picks the active one (or none, if the array is empty / nothing is marked active)
// and fetches its full row set to filter instance-type choices by what a real
// pinned snapshot can actually price. Read-only — this app never writes pricing data.
export function listPricingSnapshots(baseURL: string = ASSESSD_BASE_URL): Promise<PricingSnapshot[]> {
  return getJSON(`${baseURL}/pricing/snapshots`);
}

export function getPricingSnapshot(id: string, baseURL: string = ASSESSD_BASE_URL): Promise<PricingSnapshot> {
  return getJSON(`${baseURL}/pricing/snapshots/${id}`);
}

async function postJSON<T>(url: string, body: unknown): Promise<T> {
  const res = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const text = await res.text();
    // A real assessd always sends {error_code, message} JSON for a classified
    // failure (server/apierror.go) — but this parse is defensive, not assumed: an
    // older/differently-versioned server, or a failure that never reached
    // writeError at all (a raw net/http 500, a proxy's own error page), sends plain
    // text instead. Falling back to a bare Error for anything that doesn't parse
    // keeps every existing catch site (which only ever read err.message) working
    // unchanged.
    try {
      const parsed = JSON.parse(text) as { error_code?: string; message?: string };
      if (parsed.error_code && parsed.message) {
        throw new APIError(res.status, parsed.error_code, parsed.message);
      }
    } catch (parseErr) {
      if (parseErr instanceof APIError) throw parseErr;
      // fall through to the generic error below — text wasn't {error_code, message} JSON
    }
    throw new Error(`${res.status} ${res.statusText}: ${text}`);
  }
  return res.json() as Promise<T>;
}

// assessCanvas posts the current CanvasDocument to POST /sessions/{id}/canvas
// (PC-86), together with a real inline Workload (PC-87's own NFR form output —
// server/canvas.go's AssessCanvasRequest.Workload) rather than a server-filesystem
// path: a browser has no meaningful way to hand assessd a WorkloadPath, since there is
// no client-writable path on the SERVER's own disk that would mean anything.
export function assessCanvas(
  sessionID: string,
  canvas: CanvasDocument,
  workload: Workload,
  baseURL: string = ASSESSD_BASE_URL,
): Promise<AssessResponse> {
  return postJSON(`${baseURL}/sessions/${sessionID}/canvas`, {
    canvas,
    workload,
  });
}

// simulateNodeLoss posts {session_id, version_number, faults: [{type: "node_loss",
// target}]} to POST /simulate (core.Simulate's node_loss fault type, added for this
// ticket — see core/simulate.go). No client-side fault logic: the canvas only ever
// names WHICH node, never decides what breaks.
export function simulateNodeLoss(
  sessionID: string,
  versionNumber: number,
  targetNodeID: string,
  baseURL: string = ASSESSD_BASE_URL,
): Promise<SimulateResponse> {
  return postJSON(`${baseURL}/simulate`, {
    session_id: sessionID,
    version_number: versionNumber,
    faults: [{ type: "node_loss", target: targetNodeID }],
  });
}
