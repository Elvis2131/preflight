package core

// DetectionState is CLAUDE.md §10's FM-xxx field, exactly as specified: "detection
// (modeled | declared | unknown | observed)". PC-17's second acceptance criterion is
// verbatim about this field: it must only ever take one of these four values, never a
// boolean — the LLM narrative layer (reason/, ADR-005/I3) must never collapse it back
// into a yes/no when writing recommendations.
//
// CORRECTION, recorded rather than silently overwritten: an earlier version of this
// file had a field also named "detection" (Go type DetectionMethod), but with an
// entirely different, invented enum (structural | compliance_rule | simulation |
// live_observation) answering a different question ("how was this Finding computed"),
// written before CLAUDE.md §10's real text was available. That guess is retired, not
// kept alongside this one — two same-named, different-meaning "detection" concepts on
// one Finding would be exactly the kind of confusion this project's honesty standard
// exists to prevent. If "how was this finding computed" turns out to be a real,
// separately-needed fact, it deserves its own, differently-named field later.
type DetectionState string

const (
	// DetectionModeled: this simulator models the failure directly (e.g. PC-14's
	// fault injection/containment engines).
	DetectionModeled DetectionState = "modeled"
	// DetectionDeclared: the user/workload declared how they'd detect it (e.g. a
	// stated monitoring or runbook fact) — not verified by this simulator.
	DetectionDeclared DetectionState = "declared"
	// DetectionUnknown: no detection mechanism is known or declared.
	DetectionUnknown DetectionState = "unknown"
	// DetectionObserved: confirmed via a real experiment (validation ladder Rung 1/3).
	DetectionObserved DetectionState = "observed"
)

// EvidenceRef points at what backs a finding — an IR element (structural/compliance
// findings) or a validation-ladder run (simulated/observed findings) — "linked by ID"
// in the same spirit as ADR's ControlsSatisfied/FailureModesAddressed, so evidence is
// never an embedded copy that can drift from its source.
type EvidenceRef struct {
	NodeID       *string `json:"node_id,omitempty" jsonschema:"description=Set when this evidence points at an IR node."`
	EdgeID       *string `json:"edge_id,omitempty" jsonschema:"description=Set when this evidence points at an IR edge."`
	ExperimentID *string `json:"experiment_id,omitempty" jsonschema:"description=Set when this evidence is a validate/ Rung 1/3 run, not a structural graph reference."`
	Description  string  `json:"description" validate:"required" jsonschema:"required,minLength=1"`

	// Attribute is PC-18's own acceptance criterion, verbatim: "every finding includes
	// resource address, attribute, and rationale." NodeID is the resource address
	// (already existed); Description is the rationale (already existed); this is the
	// one genuinely new piece — the specific field a compliance check evaluated (e.g.
	// "storage_encrypted"). Optional: a structural/SPOF finding's evidence is about a
	// whole resource or edge, not one attribute, so this stays empty there.
	Attribute *string `json:"attribute,omitempty" jsonschema:"description=The specific attribute this evidence concerns, e.g. storage_encrypted — set by compliance checks, not required for structural evidence."`
}

// Recoverability is CLAUDE.md §10's own two-part phrasing: "does a failover path
// exist; is RTO/RPO feasible". Both sub-questions reuse PC-14's own functions exactly
// (core.RTOFeasibility answers "does a failover path exist" — see its own doc comment
// for why it never predicts a duration; core.RPOFeasibility answers the RPO half) —
// this is not a new derivation, it is those two results carried into the four-
// dimension shape verbatim.
type Recoverability struct {
	FailoverPathExists AssessmentEnvelope `json:"failover_path_exists" validate:"required" jsonschema:"required"`
	RPOFeasible        AssessmentEnvelope `json:"rpo_feasible" validate:"required" jsonschema:"required"`
}

// FailureMode is CLAUDE.md §10's FM-xxx record, and Finding.Dimensions' real, frozen
// shape — replacing the untyped map[string]any placeholder PC-7 left explicitly for
// PC-17 to fill in (contracts/CHANGELOG.md names this as an expected breaking change).
//
// PC-17's first acceptance criterion — "no code path can produce a single combined
// severity number" — is why Impact/Likelihood/Detectability/Recoverability are four
// SEPARATE fields, each independently not_assessable-capable, with no numeric field
// anywhere on this type and no function anywhere in this codebase that combines them.
// CLAUDE.md §10 states the reasoning directly: "a large-blast-radius/rare-trigger
// failure is not comparable to a small-blast-radius/common one — collapsing them loses
// exactly the information a reviewer needs."
type FailureMode struct {
	Trigger string `json:"trigger" validate:"required" jsonschema:"required,minLength=1"`

	// AffectedComponents and BlastRadius are both node IDs, linked by ID (same
	// convention as EvidenceRef/ADR's ControlsSatisfied). Distinct on purpose:
	// AffectedComponents is what directly failed; BlastRadius is the full downstream
	// consequence set a containment/reachability computation (PC-14/PC-78's
	// core.ContainmentBlastRadius or core.SimulateLoss) produces from it — a failure
	// with a large gap between the two IS the finding worth surfacing.
	AffectedComponents []string `json:"affected_components" validate:"required,min=1,dive,required" jsonschema:"required,minItems=1"`
	BlastRadius        []string `json:"blast_radius,omitempty"`

	Detection DetectionState `json:"detection" validate:"required,oneof=modeled declared unknown observed" jsonschema:"required,enum=modeled,enum=declared,enum=unknown,enum=observed"`

	ExistingMitigation string `json:"existing_mitigation,omitempty"`
	Gap                string `json:"gap,omitempty"`

	// The four dimensions. Each is independently AssessmentEnvelope-shaped: a
	// dimension that cannot be determined surfaces as not_assessable with a stated
	// reason, never a fabricated value and never silently omitted.
	Impact         AssessmentEnvelope `json:"impact" validate:"required" jsonschema:"required"`
	Likelihood     AssessmentEnvelope `json:"likelihood" validate:"required" jsonschema:"required,description=Per CLAUDE.md §10: assumed or unknown, never fabricated — usually not_assessable for static/structural analysis, and that is the correct, expected outcome, not a gap to fill."`
	Detectability  AssessmentEnvelope `json:"detectability" validate:"required" jsonschema:"required,description=Derived from Detection."`
	Recoverability Recoverability     `json:"recoverability" validate:"required" jsonschema:"required"`

	Recommendation string `json:"recommendation,omitempty"`
}

// Validate checks this FailureMode against the same struct tags this type is
// generated into finding.schema.json from.
func (fm FailureMode) Validate() error {
	return validate.Struct(fm)
}

// Finding is Design §5's finding.schema.json contract: "Four-dimension failure model,
// typed detection, evidence refs, not_assessable."
type Finding struct {
	ID    string `json:"id" validate:"required" jsonschema:"required,minLength=1"`
	Title string `json:"title" validate:"required" jsonschema:"required,minLength=1"`

	// Dimensions is now FailureMode (PC-17) — was an untyped map[string]any placeholder
	// under PC-7; see contracts/CHANGELOG.md for the version history of this field.
	Dimensions FailureMode `json:"dimensions" validate:"required" jsonschema:"required"`

	Evidence []EvidenceRef `json:"evidence" validate:"required,min=1,dive" jsonschema:"required,minItems=1,description=A finding must always cite at least one piece of evidence — this project's stated principle is compliance WITH evidence, never an unevidenced claim."`

	// Outcome is where I4 applies to the finding's own overall resolution (e.g. "could
	// this finding be computed at all", independent of any single dimension within
	// Dimensions being not_assessable) — a finding can have Outcome.State=assessed
	// while individual dimensions inside Dimensions are separately not_assessable;
	// the two are not the same fact.
	Outcome AssessmentEnvelope `json:"outcome" validate:"required" jsonschema:"required"`
}

// Validate checks this Finding against the same struct tags contracts/
// finding.schema.json is generated from.
func (f Finding) Validate() error {
	return validate.Struct(f)
}
