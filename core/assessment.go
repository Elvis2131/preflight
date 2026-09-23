package core

// state distinguishes the two ways an Assessment[T] can resolve. It is unexported: no
// package outside core can construct one directly, so the only way to produce an
// Assessment[T] is through Assessed or NotAssessable below.
type state int

const (
	stateAssessed state = iota
	stateNotAssessable
)

// Assessment is the sum type behind invariant I4: "incompleteness surfaces as
// not_assessable, never as pass or fail". There is deliberately no bare bool field
// anywhere on this type — Design §1 phrases it as Assessment[T] = Assessed(T) |
// NotAssessable(reason), and a bool would let a caller collapse "we don't know" into
// "false", which is exactly the failure mode I4 exists to prevent.
//
// Note what T is *not*: T is the payload of a successful assessment (a computed value,
// a rendered finding), not a pass/fail verdict. A finding's own severity/status lives in
// its own typed field, produced only when this Assessment resolved to Assessed.
//
// Assessment[T] is generic, like Tagged[T] — see AssessmentEnvelope for its wire-contract
// counterpart.
type Assessment[T any] struct {
	state  state
	value  T
	reason string // populated only when state == stateNotAssessable
	prov   Provenance
}

// Assessed constructs a resolved assessment. Per I2, the result itself carries
// provenance — an assessment about *why* this could be assessed traces back to what made
// it possible (e.g. "stated" workload data plus "derived" IR was sufficient).
func Assessed[T any](value T, prov Provenance) Assessment[T] {
	if err := prov.Validate(); err != nil {
		panic("core: Assessed requires a complete, valid Provenance: " + err.Error())
	}
	return Assessment[T]{state: stateAssessed, value: value, prov: prov}
}

// NotAssessable constructs an unresolved assessment. reason is required and non-empty:
// I4 is not satisfied by a type that merely *can* express "unknown" — every unknown must
// be a stated, inspectable reason (e.g. "resolution state unresolved: dynamic block not
// in v1 parser subset"), never a bare sentinel.
func NotAssessable[T any](reason string, prov Provenance) Assessment[T] {
	if reason == "" {
		panic("core: NotAssessable requires a non-empty reason")
	}
	if err := prov.Validate(); err != nil {
		panic("core: NotAssessable requires a complete, valid Provenance: " + err.Error())
	}
	return Assessment[T]{state: stateNotAssessable, reason: reason, prov: prov}
}

// IsAssessed reports whether this assessment resolved to a value. Callers MUST check
// this (or use Value's ok return) before trusting Value — there is no method that
// returns T alone, precisely so a caller cannot accidentally treat an unresolved
// assessment's zero value as a real result.
func (a Assessment[T]) IsAssessed() bool { return a.state == stateAssessed }

// Value returns the assessed value and true, or the zero value and false if this
// assessment is NotAssessable. Modeled on Go's comma-ok idiom deliberately: it is the
// same shape as a map lookup or a type assertion, so "check ok before using the value"
// is a pattern Go callers already have muscle memory for.
func (a Assessment[T]) Value() (T, bool) {
	if a.state != stateAssessed {
		var zero T
		return zero, false
	}
	return a.value, true
}

// Reason returns the not-assessable reason and true, or "" and false if this assessment
// resolved to a value.
func (a Assessment[T]) Reason() (string, bool) {
	if a.state != stateNotAssessable {
		return "", false
	}
	return a.reason, true
}

func (a Assessment[T]) Provenance() Provenance { return a.prov }

// ToEnvelope converts this Assessment[T] to its wire-contract shape.
func (a Assessment[T]) ToEnvelope() AssessmentEnvelope {
	env := AssessmentEnvelope{Provenance: a.prov}
	if a.state == stateAssessed {
		env.State = AssessmentStateAssessed
		env.Value = a.value
	} else {
		env.State = AssessmentStateNotAssessable
		env.Reason = a.reason
	}
	return env
}

// AssessmentState is the wire-level enum for AssessmentEnvelope.State.
type AssessmentState string

const (
	AssessmentStateAssessed      AssessmentState = "assessed"
	AssessmentStateNotAssessable AssessmentState = "not_assessable"
)

// AssessmentEnvelope is the wire-contract counterpart to Assessment[T] (Design §5:
// provenance.schema.json governs "the five-value enum and the Tagged/Assessment
// wrappers"). Reason is required exactly when State is not_assessable and forbidden
// otherwise — validator's required_if/excluded_if enforce that pairing structurally, the
// same guarantee Assessment[T]'s Go API gives internally, now expressed as a schema a
// non-Go caller (a stored session, an HTTP response) can also be checked against.
type AssessmentEnvelope struct {
	State      AssessmentState `json:"state" validate:"required,oneof=assessed not_assessable" jsonschema:"required,enum=assessed,enum=not_assessable"`
	Value      any             `json:"value,omitempty" validate:"required_if=State assessed,excluded_if=State not_assessable" jsonschema:"description=Present iff state is assessed."`
	Reason     string          `json:"reason,omitempty" validate:"required_if=State not_assessable,excluded_if=State assessed" jsonschema:"description=Present iff state is not_assessable — I4: incompleteness always carries a stated reason, never a bare sentinel."`
	Provenance Provenance      `json:"provenance" validate:"required" jsonschema:"required"`
}

// Validate checks this envelope against the same struct tags AssessmentEnvelope's
// contract schema is generated from — see Provenance.Validate for when to use this
// (untrusted/deserialized input) versus the panic-based Go constructors (programming
// errors inside core).
func (e AssessmentEnvelope) Validate() error {
	return validate.Struct(e)
}
