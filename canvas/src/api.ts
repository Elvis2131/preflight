// api.ts is the ONLY place this app talks to assessd (P1) over HTTP — PC-88's own
// scope note: "the canvas never computes a fault result itself." Every type below
// mirrors a real Go response shape by field name, not guessed — cross-checked against
// server/canvas.go's AssessResponse and core/simulate.go's SimulateResponse/
// AssessmentEnvelope before being typed here.
import type { CanvasDocument } from "./types";
import type { Workload } from "./workloadTypes";

// Overridable for a dev machine where 8080 is taken (VITE_ASSESSD_URL=http://localhost:8099).
export const ASSESSD_BASE_URL: string = import.meta.env.VITE_ASSESSD_URL ?? "http://localhost:8080";

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

// JourneyHopFlow/JourneyFlowResult mirror core.JourneyHopFlow/core.JourneyFlowResult
// (core/journey_flow.go, PC-125) — no json tags on the Go structs, so field names are
// the default Go-encoding/json PascalCase, not guessed or snake_cased. GroupIndex/
// ReachedByGroup (PC-125/126's own follow-up) expose "|"-separated parallel Path
// groups: GroupIndex is the destination's own position in Path; ReachedByGroup[i] is
// the sorted list of Path[i]'s own declared members actually reached.
export interface JourneyHopFlow {
  From: string;
  To: string;
  Allowed: boolean;
  Reason: string;
  GroupIndex: number;
}

export interface JourneyFlowResult {
  JourneyID: string;
  Flows: boolean;
  Hops: JourneyHopFlow[];
  ReachedByGroup: string[][];
  BlockedAt: string;
  BlockedReason: string;
  // PC-131: set only when the primary path does not flow but the journey's own
  // declared fallback does (omitted from the wire otherwise). Never computed here.
  Degraded?: boolean;
  DegradedVia?: string;
}

// ComponentLoad mirrors core.ComponentLoad (core/load.go, PC-126) — again no json
// tags, so PascalCase. Capacity/Utilization are nullable exactly as the Go doc
// comment states: nil means undeclared/not_assessable, never a guessed default.
// LoadDivisionNote (PC-126's own follow-up) is non-empty only for a component that is
// actually a member of a currently-reached parallel group of more than one.
export interface ComponentLoad {
  NodeID: string;
  NodeType: string;
  OfferedRPS: number;
  CapacityKey: string;
  Capacity: number | null;
  Utilization: number | null;
  NotAssessableReason: string;
  LoadDivisionNote: string;
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
  // flow_detail/load (PC-127): PC-125/126's own real per-journey flow and
  // per-component utilization, exposed so this UI never recomputes either
  // client-side — see core/simulate.go's own SimulateResponse doc comment.
  flow_detail: JourneyFlowResult[];
  load: ComponentLoad[];
  // latency (PC-128): Layer 3 per-journey latency/saturation, assessed (a conditional
  // statement + the declared inputs it was computed from) or not_assessable naming
  // what is missing. Shown verbatim — never recomputed here.
  latency?: JourneyLatency[];
}

export interface JourneyLatencyEstimate {
  statement: string;
  mean_ms?: number;
  saturated_at?: string;
  inputs: Array<{ name: string; value: number; source: string }>;
  assumptions: string[];
  seed: string;
}

export interface JourneyLatency {
  journey_id: string;
  result: AssessmentEnvelope;
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

// ServiceCatalogEntry mirrors server.ServiceCatalogEntry (server/catalog.go, PC-136)
// field-for-field — the real merged providers.Registry's own node mappings (edge
// mappings excluded server-side), never an invented list of AWS/Azure services.
export interface ServiceCatalogEntry {
  resource_type: string;
  node_type: string;
  capability_level: string;
}

// listServiceCatalog (PC-136) backs the Inspector's own Service picker: GET
// /catalog/services returns every real node mapping so a canvas node's ServiceID can
// be chosen from services this engine actually resolves, filtered client-side by the
// selected node's own NodeType (Inspector.tsx) — never a second copy of the registry
// data, only a read of what the server already computed from it.
export function listServiceCatalog(baseURL: string = ASSESSD_BASE_URL): Promise<ServiceCatalogEntry[]> {
  return getJSON(`${baseURL}/catalog/services`);
}

// listVersions (PC-123) backs the Report mode's own version picker — GET
// /sessions/{id}/versions (PC-92) returns one AssessResponse-shaped entry per stored
// version; only version_number is actually read by the picker today.
export interface VersionSummary {
  version_number: number;
}
export function listVersions(sessionID: string, baseURL: string = ASSESSD_BASE_URL): Promise<VersionSummary[]> {
  return getJSON(`${baseURL}/sessions/${sessionID}/versions`);
}

// Report mirrors core.Report (preflight/core/report.go) field-for-field, cross-checked
// against that Go source before typing, not guessed — PC-120's own seventh frozen
// contract (report.schema.json). Nested value shapes the Report view never inspects
// beyond "does it exist / what's its own display text" are typed loosely
// (Record<string, unknown> / unknown[]) rather than fully mirrored, since a UI that
// only ever displays server-provided fields verbatim (never computes from them) has
// no need to know their full internal shape — the same "thin, computes nothing"
// boundary this ticket's own Card requires.
export interface ComplianceCatalogCounts {
  Assessable: number;
  Partial: number;
  NotAssessable: number;
}
export interface ComplianceResultCounts {
  Satisfied: number;
  Applicable: number;
  Partial: number;
  Unsatisfied: number;
  NotAssessable: number;
}
export interface ReportExecutiveSummary {
  scorecard_status_counts: Record<string, number>;
  compliance_catalog_counts: Record<string, ComplianceCatalogCounts>;
  compliance_result_counts: Record<string, ComplianceResultCounts>;
  nfr_evaluated_count: number;
  nfr_not_evaluated_count: number;
}
export interface ReportNFREntry {
  requirement_id: string;
  priority: string;
  value: unknown;
  evaluated: boolean;
  finding_ids?: string[];
  reason?: string;
}
export interface ReportComplianceFrameworkSection {
  framework: string;
  catalog_counts: ComplianceCatalogCounts;
  result_counts: ComplianceResultCounts;
  controls: Array<{
    ControlID: string;
    RequirementID: string;
    Title: string;
    Classification: string;
    NodeID?: string;
    Result: { status: string; provenance: unknown };
    Rationale: string;
  }>;
}
export interface ReportFailureModesSection {
  available: boolean;
  unavailable_reason?: string;
  findings?: Array<{ id: string; title: string; dimensions: Record<string, unknown>; outcome: { state: string; value?: unknown; reason?: string } }>;
}
export interface ReportTrafficSection {
  available: boolean;
  unavailable_reason?: string;
  flows?: Array<{ JourneyID: string; Flows: boolean; BlockedAt?: string; BlockedReason?: string }>;
  load?: unknown[];
}
export interface UsageBasedCostEntry {
  JourneyID: string;
  Kind: string;
  Decision: string;
  MonthlyAmount: number;
  Currency: string;
  Reason: string;
}
export interface ReportCostSection {
  available: boolean;
  unavailable_reason?: string;
  disclaimer: string;
  report?: {
    SnapshotID: string;
    PricedTotal: number;
    Currency: string;
    UnpricedCount: number;
    Components: Array<{
      NodeID: string;
      Decision: string;
      MonthlyAmount: number;
      Currency: string;
      Reason: string;
      // PC-110: the region this component was priced in and which resolution step supplied
      // it (component | workload) — absent when no region resolved.
      Region?: string;
      RegionSource?: string;
    }>;
  };
  usage_based_charges: UsageBasedCostEntry[];
}
export interface DeltaEntry {
  finding_id: string;
  kind: string;
  old_status?: string;
  new_status?: string;
  reason?: string;
}
export interface ReportAssumption {
  kind: string;
  source: string;
  reason?: string;
}
export interface Report {
  session_id: string;
  version_number: number;
  graph: string;
  executive_summary: ReportExecutiveSummary;
  inventory: Array<{ node_id: string; type: string; resolution: string }>;
  nfr_conformance: ReportNFREntry[];
  compliance: ReportComplianceFrameworkSection[];
  failure_modes: ReportFailureModesSection;
  traffic: ReportTrafficSection;
  cost: ReportCostSection;
  delta: DeltaEntry[];
  // scenarios (PC-131): saved Failure Lab scenarios evaluated by the server against THIS
  // report's version — shown verbatim, never recomputed here.
  scenarios?: ScenarioResult[];
  assumptions: ReportAssumption[];
}

// getReport (PC-123) fetches the JSON report — the "generate and display" half of
// Report mode. format is a thin passthrough to the same query param the server
// already understands (PC-122); this function never renders anything itself.
export function getReport(sessionID: string, versionNumber: number, baseURL: string = ASSESSD_BASE_URL): Promise<Report> {
  return getJSON(`${baseURL}/sessions/${sessionID}/versions/${versionNumber}/report`);
}

// reportExportURL (PC-123) builds the URL for the HTML/PDF export links — a plain
// string, not a fetch: the browser's own "open in new tab" / native download handling
// is the export mechanism, this app performs no client-side rendering of either
// format (that would duplicate PC-122's own server-side renderer).
export function reportExportURL(sessionID: string, versionNumber: number, format: "html" | "pdf", baseURL: string = ASSESSD_BASE_URL): string {
  return `${baseURL}/sessions/${sessionID}/versions/${versionNumber}/report?format=${format}`;
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

// simulateBaseline (PC-127) posts an empty faults list — core.Simulate/resolveFaults
// treats that as a real, legitimate no-fault scenario (an empty killed set), not a
// special case this file invents. Gives the journey/utilization panel a genuine
// server-computed "before" to compare a fault's "after" against, rather than
// synthesizing one client-side.
export function simulateBaseline(
  sessionID: string,
  versionNumber: number,
  baseURL: string = ASSESSD_BASE_URL,
): Promise<SimulateResponse> {
  return postJSON(`${baseURL}/simulate`, {
    session_id: sessionID,
    version_number: versionNumber,
    faults: [],
  });
}

// Reference-architecture templates (PC-108): pure reads of checked-in data. Loading one
// into the canvas and assessing it uses the ordinary assessCanvas path — there is no
// "assess template" endpoint, so no template-specific code path can exist.
export interface TemplateMeta {
  id: string;
  name: string;
  description: string;
}

export interface Template {
  meta: TemplateMeta;
  canvas: CanvasDocument;
  workload: Workload;
  // UI-only node placement — never part of the frozen canvas contract.
  layout: Record<string, { x: number; y: number; width?: number; height?: number }>;
}

export async function listTemplates(): Promise<TemplateMeta[]> {
  const res = await fetch(`${ASSESSD_BASE_URL}/templates`);
  if (!res.ok) throw new Error(`listTemplates: HTTP ${res.status}`);
  return res.json();
}

export async function getTemplate(id: string): Promise<Template> {
  const res = await fetch(`${ASSESSD_BASE_URL}/templates/${encodeURIComponent(id)}`);
  if (!res.ok) throw new Error(`getTemplate(${id}): HTTP ${res.status}`);
  return res.json();
}

// Failure Lab scenarios (PC-131). A fault is NAMED here (type + target, plus the exact
// rule for a rule removal); what it breaks is the server's decision. Field names of the
// rule mirror core.SGRule, which has no json tags.
export interface FaultSGRule {
  Direction: string;
  Protocol: string;
  FromPort: number;
  ToPort: number;
  CIDRs?: string[];
  SourceSG?: string;
}

export interface Fault {
  type: string;
  target: string;
  sg_rule_remove?: FaultSGRule;
}

export interface SavedScenario {
  name: string;
  faults: Fault[];
}

export interface ScenarioResult extends SavedScenario {
  verdict: AssessmentEnvelope;
  severed_paths: string[];
  cascade: string[];
  failed_journeys: string[];
  degraded_journeys: string[];
}

export function simulateFaults(
  sessionID: string,
  versionNumber: number,
  faults: Fault[],
  baseURL: string = ASSESSD_BASE_URL,
): Promise<SimulateResponse> {
  return postJSON(`${baseURL}/simulate`, { session_id: sessionID, version_number: versionNumber, faults });
}

export function listScenarios(sessionID: string, baseURL: string = ASSESSD_BASE_URL): Promise<SavedScenario[]> {
  return getJSON(`${baseURL}/sessions/${sessionID}/scenarios`);
}

export async function saveScenario(sessionID: string, sc: SavedScenario, baseURL: string = ASSESSD_BASE_URL): Promise<SavedScenario> {
  const res = await fetch(`${baseURL}/sessions/${sessionID}/scenarios/${encodeURIComponent(sc.name)}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ faults: sc.faults }),
  });
  if (!res.ok) throw new Error(`saveScenario: ${res.status} ${await res.text()}`);
  return res.json();
}

export async function deleteScenario(sessionID: string, name: string, baseURL: string = ASSESSD_BASE_URL): Promise<void> {
  const res = await fetch(`${baseURL}/sessions/${sessionID}/scenarios/${encodeURIComponent(name)}`, { method: "DELETE" });
  if (!res.ok && res.status !== 404) throw new Error(`deleteScenario: ${res.status} ${await res.text()}`);
}

// evaluateScenarios asks the server to re-run every saved scenario against one stored
// version, now — never a stored result.
export function evaluateScenarios(sessionID: string, versionNumber: number, baseURL: string = ASSESSD_BASE_URL): Promise<ScenarioResult[]> {
  return getJSON(`${baseURL}/sessions/${sessionID}/versions/${versionNumber}/scenarios`);
}
