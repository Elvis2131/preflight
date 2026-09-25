// Package core is the pure assessment engine (invariant I1, CLAUDE.md §4): no I/O, no
// network, no LLM, no provider names, no clock, no randomness without an injected seed.
// It is the only public surface onto core/internal/{ir,analyse,simulate} — those packages
// are compiler-enforced private to this subtree, so ingest/, reason/, server/ and
// providers/ can only reach them through the pure API this package exposes.
package core

import "github.com/go-playground/validator/v10"

// validate is shared across every contract type in this package (PC-7: "each schema is
// generated from the same Go structs the code validates against" — one validator
// instance, one set of struct tags, used both to reject bad construction at the Go
// boundary and to generate contracts/*.schema.json via invopop/jsonschema).
var validate = validator.New(validator.WithRequiredStructEnabled())

func init() {
	validate.RegisterStructValidation(validateProvenanceEvidenceTierRung, Provenance{})
}

// validateProvenanceEvidenceTierRung is I5's OTHER half, previously described only in
// EvidenceTierPerformance/EvidenceTierResilience's own doc comments ("requires a real
// or sufficiently faithful environment, never Rung 2", "requires Rung 3 ... no emulated
// or replica environment can produce this tier") but never actually enforced anywhere
// — PC-24's own acceptance criterion is exactly this gap: "observed results are
// rejected by any performance field at the schema level ... tested against this
// specific harness's output, not just assumed to work." validate.go's existing
// required_if/excluded_unless tags only enforce that EvidenceTier+Rung are present
// together when Kind is observed; they say nothing about which Rung may carry which
// tier, which is the actual laundering PRD §5.6 names ("Rung-2 (emulated) results are
// typed functional and structurally rejected by performance/resilience fields").
//
// Only Rung 3 (a real ephemeral cloud experiment) may carry EvidenceTierPerformance or
// EvidenceTierResilience: Rung 1 (a local topology replica) and Rung 2 (LocalStack
// emulation) can only ever produce EvidenceTierFunctional observations — that ceiling
// is exactly what stops a Rung 1/2 result from ever backing a throughput/latency/RTO/
// RPO claim it never earned.
func validateProvenanceEvidenceTierRung(sl validator.StructLevel) {
	p := sl.Current().Interface().(Provenance)
	if p.Kind != KindObserved || p.EvidenceTier == nil || p.Rung == nil {
		return
	}
	if *p.EvidenceTier != EvidenceTierFunctional && *p.Rung != Rung3RealCloud {
		sl.ReportError(p.EvidenceTier, "EvidenceTier", "EvidenceTier", "tier_exceeds_rung", "")
	}
}

// Kind is the five-value provenance enum — PRD §4's exact wording: "every field in every
// response carries a source tag: stated (from the workload declaration or user input),
// derived (deterministic computation over the IR), assumed (a declared default the user
// can override), llm_reasoned (LLM contextual judgment, always citing IR evidence),
// observed (measured in a real experiment)." The schema makes untagged assertions
// unrepresentable — that sentence is I2, and this type is the mechanism.
type Kind string

const (
	KindStated      Kind = "stated"
	KindDerived     Kind = "derived"
	KindAssumed     Kind = "assumed"
	KindLLMReasoned Kind = "llm_reasoned"
	KindObserved    Kind = "observed"
)

// EvidenceTier is I5's mechanism: "Evidence tier cannot be laundered" — "Rung-2
// (emulated) results are typed functional and structurally rejected by
// performance/resilience fields." Only meaningful when Kind == KindObserved; it answers
// what class of claim this observation is strong enough to support, independent of
// which validation-ladder rung produced it.
type EvidenceTier string

const (
	// EvidenceTierFunctional supports "did this succeed/fail as expected" claims only.
	EvidenceTierFunctional EvidenceTier = "functional"
	// EvidenceTierPerformance supports throughput/latency claims — requires a real or
	// sufficiently faithful environment, never Rung 2 (LocalStack emulation).
	EvidenceTierPerformance EvidenceTier = "performance"
	// EvidenceTierResilience supports RTO/RPO/failover-time claims — requires Rung 3
	// (a real ephemeral cloud experiment); no emulated or replica environment can
	// produce this tier, because failover timing is exactly the thing emulation fakes.
	EvidenceTierResilience EvidenceTier = "resilience"
)

// Rung identifies which validation-ladder tier (CLAUDE.md §6, `validate/`) produced an
// observed value: 1 = Toxiproxy/kind topology replica, 2 = LocalStack (emulated cloud
// APIs), 3 = ephemeral apply/experiment/destroy against a real cloud account.
type Rung int

const (
	Rung1TopologyReplica Rung = 1
	Rung2Emulated        Rung = 2
	Rung3RealCloud       Rung = 3
)

// Provenance answers "where did this value come from" for exactly one Tagged value. It
// is not optional metadata bolted onto a result — it is the mechanism that makes I2
// ("every assertion carries provenance") a property of the type system rather than a
// convention someone can forget to follow.
//
// Fields are exported so this struct doubles as the wire contract PC-7 requires
// (provenance.schema.json is generated from this type via invopop/jsonschema, not
// hand-written) — but exporting the fields does not weaken I2: NewProvenance/NewTagged/
// Assessed/NotAssessable all reject the zero value regardless of field visibility, and
// Validate() catches a non-zero-but-invalid value (e.g. a bogus Kind) that field
// visibility alone could never have caught anyway.
type Provenance struct {
	Kind Kind `json:"kind" validate:"required,oneof=stated derived assumed llm_reasoned observed" jsonschema:"required,enum=stated,enum=derived,enum=assumed,enum=llm_reasoned,enum=observed"`

	// Source names what produced this value, e.g. "aws_db_instance.payments:multi_az"
	// or "workload.yaml:availability.rto_seconds". Required: a provenance with no named
	// source is not meaningfully different from having none at all.
	Source string `json:"source" validate:"required" jsonschema:"required,minLength=1"`

	// Reason is required for Kind == assumed (PRD §4: "a declared default the user can
	// override" — the override target must be nameable) and for Kind == llm_reasoned
	// (PRD §4: "always citing IR evidence"). Optional for the other three kinds.
	Reason string `json:"reason,omitempty" validate:"required_if=Kind assumed,required_if=Kind llm_reasoned" jsonschema:"description=Required for assumed and llm_reasoned; the IR evidence citation or the overridable default's justification."`

	// EvidenceTier and Rung apply only when Kind == observed (I5). validator's
	// required_if enforces the pairing; excluded_unless keeps them absent otherwise, so
	// a non-observed value can never carry a tier it did not earn.
	EvidenceTier *EvidenceTier `json:"evidence_tier,omitempty" validate:"excluded_unless=Kind observed,required_if=Kind observed,omitempty,oneof=functional performance resilience" jsonschema:"enum=functional,enum=performance,enum=resilience"`
	Rung         *Rung         `json:"rung,omitempty" validate:"excluded_unless=Kind observed,required_if=Kind observed,omitempty,oneof=1 2 3" jsonschema:"enum=1,enum=2,enum=3"`
}

// NewProvenance constructs a Provenance, checking only Kind (a real enum value) and
// Source (non-empty) — the two fields that are always present. It deliberately does
// NOT run the cross-field rules (assumed/llm_reasoned requiring Reason; observed
// requiring EvidenceTier+Rung), because those fields are attached afterwards via
// WithReason/WithObserved: this is a builder step, not a claim of completeness.
// Full correctness is Validate()'s job, and it is what NewTagged/Assessed/
// NotAssessable actually enforce before allowing a Provenance to back a real value —
// see those constructors below.
func NewProvenance(kind Kind, source string) Provenance {
	p := Provenance{Kind: kind, Source: source}
	if err := validate.StructPartial(p, "Kind", "Source"); err != nil {
		panic("core: invalid Provenance: " + err.Error())
	}
	return p
}

// WithReason attaches an explanation, required in practice for Kind == assumed
// (PRD §4: "a declared default the user can override") and Kind == llm_reasoned
// (PRD §4: "always citing IR evidence"). Does not validate eagerly — see NewProvenance.
func (p Provenance) WithReason(reason string) Provenance {
	p.Reason = reason
	return p
}

// WithObserved attaches the evidence tier and validation-ladder rung required for
// Kind == observed (I5). Does not validate eagerly — see NewProvenance.
func (p Provenance) WithObserved(tier EvidenceTier, rung Rung) Provenance {
	p.EvidenceTier = &tier
	p.Rung = &rung
	return p
}

// Validate checks this Provenance against the same struct tags contracts/
// provenance.schema.json is generated from. Use this (not a panic) at any boundary
// where the Provenance came from outside this process — e.g. server/ deserializing a
// stored session or an incoming request — since untrusted input failing validation is
// an ordinary, expected outcome there, not a programming error.
func (p Provenance) Validate() error {
	return validate.Struct(p)
}

// IsZero reports whether this is the zero-value Provenance — i.e. one that was never
// constructed via NewProvenance. Tagged and Assessment use this to reject zero-value
// provenance at construction, which is what actually makes "untagged values are
// unconstructable" true: without this check, Go's zero-value struct literal would
// silently produce a Provenance with an empty Kind and source, defeating the guarantee.
func (p Provenance) IsZero() bool { return p == Provenance{} }

// Tagged pairs a value with its Provenance. Per I2, a Tagged[T] with no provenance must
// be unconstructable. The field is unexported so the only way to produce a non-zero
// Tagged[T] from outside this package is NewTagged, which rejects a zero Provenance.
//
// Tagged is generic for internal type-safety inside the pure engine; it is intentionally
// NOT itself one of the five frozen wire contracts (JSON Schema has no first-class
// generics). Where a Tagged[T] value crosses the process boundary (server/'s JSON
// responses), it is represented as the concrete TaggedEnvelope below, which invopop/
// jsonschema can actually reflect.
type Tagged[T any] struct {
	value T
	prov  Provenance
}

// NewTagged is the only exported constructor for Tagged[T]. It panics on a zero-value
// Provenance rather than returning an error: an untagged value is a programming error in
// the caller, not a runtime condition the caller should be routing around (contrast with
// Assessment[T], where "we don't know" is an expected, first-class outcome — see I4).
func NewTagged[T any](value T, prov Provenance) Tagged[T] {
	if err := prov.Validate(); err != nil {
		panic("core: Tagged requires a complete, valid Provenance: " + err.Error())
	}
	return Tagged[T]{value: value, prov: prov}
}

func (t Tagged[T]) Value() T               { return t.value }
func (t Tagged[T]) Provenance() Provenance { return t.prov }

// TaggedEnvelope is the wire-contract counterpart to Tagged[T] (Design §5:
// provenance.schema.json governs "the five-value enum and the Tagged/Assessment
// wrappers"). Value is untyped on purpose: this envelope is the SHARED shape every
// Tagged field in every other contract embeds; the concrete type of Value is determined
// by whichever IR/Workload/Finding field uses it, not by this schema.
type TaggedEnvelope struct {
	Value      any        `json:"value" jsonschema:"required"`
	Provenance Provenance `json:"provenance" validate:"required" jsonschema:"required"`
}
