// workloadTypes.ts mirrors core.Workload (preflight/core/workload.go) exactly — the
// same "one source of truth, mirrored deliberately, not independently redefined"
// relationship goldenVocabulary.ts already has with core.NodeType/EdgeType (PC-85),
// now extended to workload.schema.json (PC-87). Field names copied verbatim from the
// real Go struct's json tags, not retyped from memory.
export type RequirementPriority = "hard" | "preference";

export interface Requirement {
  id: string;
  // Value mirrors core.Requirement.Value (Go `any`) — PRD §4 gives no single type
  // (an RTO is a duration, an availability target a percentage, a profile membership
  // a string). The form only ever captures free text here; a plain string is a valid
  // value against workload.schema.json's own `value: true` (literally any JSON value).
  value: string;
  priority: RequirementPriority;
  // rank is required when priority is "preference", must be absent when "hard" —
  // core.Requirement's own validate tag (required_if/excluded_if), enforced here in
  // the form too (see WorkloadForm) rather than only being caught server-side.
  rank?: number;
}

// DeclaredJourney mirrors core.DeclaredJourney (PC-124) — path is a list of IR Node
// IDs (or "internet" as the first hop, core.JourneyInternetSentinel). peak_rps/
// steady_rps are optional: absent means the load engine reports not_assessable for
// that journey, never a guessed number (same discipline as Workload.capacity above).
export interface DeclaredJourney {
  id: string;
  name: string;
  path: string[];
  protocol: string;
  port: number;
  criticality: string;
  peak_rps?: number;
  steady_rps?: number;
  // PC-152: optional per-hop port overrides keyed by the hop's destination (the exact path
  // element); a hop with no entry uses `port`. Preserved and editable in the form.
  hop_ports?: Record<string, number>;
}

export interface Workload {
  schema_version: string;
  // PC-128: optional squared coefficient of variation of each component type's service time,
  // keyed like capacity (1 = exponential, 0 = regular). Not yet authorable in the form.
  service_time_scv?: Record<string, number>;
  name: string;
  criticality: string;
  data_classification: string;
  regions: string[];
  compliance_profiles: string[];
  requirements: Requirement[];
  // capacity is deliberately OPTIONAL and sparse: a node-type-scoped key absent from
  // this map means capacity_unknown for that key (core/workload.go's own doc
  // comment) — never a zero, never inferred. WorkloadForm must preserve this: a
  // blank capacity field in the form omits the key entirely, it never becomes 0.
  capacity?: Record<string, number>;
  // journeys (PC-127's own addition to the form; the field itself is PC-124's) —
  // omitted entirely when none are declared, never an empty array standing in for
  // "the architect considered this and declared nothing."
  journeys?: DeclaredJourney[];
}

export const WORKLOAD_SCHEMA_VERSION = "1.0.0";

export function emptyWorkload(): Workload {
  return {
    schema_version: WORKLOAD_SCHEMA_VERSION,
    name: "",
    criticality: "",
    data_classification: "",
    regions: [],
    compliance_profiles: [],
    requirements: [],
    capacity: {},
  };
}
