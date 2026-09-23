package core

// ComplianceStatus is PC-18's third acceptance criterion, verbatim: "no control ever
// returns a bare pass/fail — always one of applicable/satisfied/partial/unsatisfied/
// not-assessable."
//
// Design note, honestly flagged: Design §4.6 (the source this ticket cites for the
// compliance engine's shape) was not available when this was written, so the exact
// intended MEANING of "applicable" as a fifth state alongside satisfied/partial/
// unsatisfied/not_assessable is inferred, not confirmed. Two readings were considered:
// (a) "applicable" is shorthand/a typo for "not_applicable" (the control doesn't apply
// to this resource type — a natural parallel to CLAUDE.md §14/§27's "resource outside
// the vocabulary" concept, extended to compliance controls); (b) "applicable" is a
// literal, standalone state (e.g. a control-scope-level fact distinct from a
// per-resource verdict). This implementation takes the literal ticket wording at face
// value (reading b) rather than silently "correcting" it to reading (a) — inverting a
// stated requirement's own word without confirmation would be a worse mistake than
// leaving the ambiguity visible. If Design §4.6 surfaces later and confirms reading
// (a), this is a one-line fix, not a rearchitecture: ComplianceApplicable's value
// changes meaning, not its presence in the enum.
type ComplianceStatus string

const (
	// ComplianceApplicable: this control is in scope for this resource, but this
	// specific result represents scope/applicability rather than a satisfied/partial/
	// unsatisfied verdict (see the type doc comment's honesty note on this value's
	// exact intended meaning).
	ComplianceApplicable ComplianceStatus = "applicable"
	// ComplianceSatisfied: the control's requirement is met.
	ComplianceSatisfied ComplianceStatus = "satisfied"
	// CompliancePartial: the control's requirement is partially met — reserved for a
	// control with multiple sub-conditions; PC-18's single implemented control (RDS
	// storage encryption, a single boolean check) does not exercise this value, and
	// says so rather than fabricating a partial case to look complete.
	CompliancePartial ComplianceStatus = "partial"
	// ComplianceUnsatisfied: the control's requirement is violated.
	ComplianceUnsatisfied ComplianceStatus = "unsatisfied"
	// ComplianceNotAssessable: I4's usual meaning, applied to compliance specifically —
	// the relevant attribute's value could not be determined from the IR.
	ComplianceNotAssessable ComplianceStatus = "not_assessable"
)

// ComplianceResult wraps a ComplianceStatus with real Provenance — the compliance-
// engine counterpart to AssessmentEnvelope, using a 5-value status instead of the
// 2-value assessed/not_assessable state, since "no bare pass/fail" is this ticket's
// own, more specific requirement than plain I4.
type ComplianceResult struct {
	Status     ComplianceStatus `json:"status" validate:"required,oneof=applicable satisfied partial unsatisfied not_assessable" jsonschema:"required,enum=applicable,enum=satisfied,enum=partial,enum=unsatisfied,enum=not_assessable"`
	Provenance Provenance       `json:"provenance" validate:"required" jsonschema:"required"`
}

// Validate checks this ComplianceResult against the same struct tags it is generated
// from — same pattern as every other frozen-adjacent type in this package.
func (c ComplianceResult) Validate() error {
	return validate.Struct(c)
}
