package core

// RequirementPriority is PRD §4's distinction: "hard = constraint (violation is a
// failing finding), preference = ranked goal (violation trades against, not fails)."
type RequirementPriority string

const (
	PriorityHard       RequirementPriority = "hard"
	PriorityPreference RequirementPriority = "preference"
)

// Requirement is one entry in Workload.Requirements. Value is untyped: PRD §4 gives no
// single type for a requirement's value (an RTO is a duration, a compliance profile
// membership is a string, an availability target is a percentage) — inventing one fixed
// type here would be narrowing a deliberately open PRD shape, not freezing it.
type Requirement struct {
	ID       string              `json:"id" yaml:"id" validate:"required" jsonschema:"required,minLength=1"`
	Value    any                 `json:"value" yaml:"value" validate:"required" jsonschema:"required"`
	Priority RequirementPriority `json:"priority" yaml:"priority" validate:"required,oneof=hard preference" jsonschema:"required,enum=hard,enum=preference"`

	// Rank orders preference requirements against each other. PRD §4: "rank (for
	// preference only)". excluded_if enforces "only" in the schema, not just in prose.
	Rank *int `json:"rank,omitempty" yaml:"rank,omitempty" validate:"required_if=Priority preference,excluded_if=Priority hard" jsonschema:"description=Required when Priority is preference; must be absent when Priority is hard."`
}

// DeclaredJourney is PC-124's own declaration: one user-facing request flow through
// the architecture, with its own expected load — "checkout: internet → WAF → ALB →
// ECS → RDS" is the Card's own example. Named distinctly from core.Journey
// (core/simulate.go) — that type is PC-82's own COMPUTED /simulate response fact
// ("did this entry-point-to-target path survive the fault"), a different concept
// that happens to share the English word; this one is the architect-DECLARED input
// PC-14/82's Journey computes reachability over. Reuses Workload.Capacity for
// per-component capacity rather than adding a second capacity concept here (the
// Card's own explicit instruction) — see JourneyCapacityKey's own doc comment for the
// key convention this ticket establishes for that lookup.
type DeclaredJourney struct {
	ID   string `json:"id" yaml:"id" validate:"required,min=1" jsonschema:"required,minLength=1"`
	Name string `json:"name" yaml:"name" validate:"required,min=1" jsonschema:"required,minLength=1"`

	// Path is the ordered component path this journey travels — IR Node.IDs. At
	// minimum a source and a destination (PC-114's BuildTrace resolves the route
	// between them); an architect may declare the full intermediate path instead
	// when they want to be explicit about it, per the Card's own "(or source +
	// destination for the request engine to resolve)".
	Path []string `json:"path" yaml:"path" validate:"required,min=2,dive,required" jsonschema:"required,minItems=2"`

	Protocol    string `json:"protocol" yaml:"protocol" validate:"required" jsonschema:"required"`
	Port        int    `json:"port" yaml:"port" validate:"required" jsonschema:"required"`
	Criticality string `json:"criticality" yaml:"criticality" validate:"required" jsonschema:"required,description=e.g. tier1 — the same workload-declared vocabulary as Workload.Criticality, not a fixed enum."`

	// PeakRPS/SteadyRPS are pointers, same "absence is a real, stated gap, never
	// assumed as zero" discipline as every other optional declared value in this
	// schema (Requirement.Rank, CapabilityModel's own fields). A journey missing
	// either still gets structural flow (PC-125); load results are not_assessable
	// (EvaluateJourneyLoadReadiness, core/journey.go), never a guessed number.
	PeakRPS   *float64 `json:"peak_rps,omitempty" yaml:"peak_rps,omitempty"`
	SteadyRPS *float64 `json:"steady_rps,omitempty" yaml:"steady_rps,omitempty"`
}

// Workload is PRD §4's required input: "name, criticality, data_classification,
// regions[], compliance_profiles[]" plus requirements and declared capacity.
//
// Capacity is a map keyed by node-type-scoped capacity name (PRD §4's own example:
// "app_node_rps"). PRD §4 is explicit about how absence must be read: "No declared
// capacity → capacity_unknown → any capacity finding is not_assessable, never assumed,
// never zero." That rule is why Capacity has no sentinel "unknown" value anywhere in
// this schema — a node-type simply absent from the map IS capacity_unknown. Adding an
// explicit "unknown" enum value here would create a second way to express the same
// thing and risk the two silently diverging (a caller checks the wrong one).
type Workload struct {
	SchemaVersion string `json:"schema_version" yaml:"schema_version" validate:"required" jsonschema:"required"`

	Name               string   `json:"name" yaml:"name" validate:"required" jsonschema:"required,minLength=1"`
	Criticality        string   `json:"criticality" yaml:"criticality" validate:"required" jsonschema:"required,description=e.g. tier1 — the golden reference architecture's own vocabulary (CLAUDE.md §14), not a fixed enum here since criticality tiers are workload-declared, not IR-canonical."`
	DataClassification string   `json:"data_classification" yaml:"data_classification" validate:"required" jsonschema:"required"`
	Regions            []string `json:"regions" yaml:"regions" validate:"required,min=1,dive,required" jsonschema:"required,minItems=1"`
	ComplianceProfiles []string `json:"compliance_profiles" yaml:"compliance_profiles" jsonschema:"description=e.g. PCI — may be empty; not every workload has a stated compliance profile."`

	Requirements []Requirement `json:"requirements" yaml:"requirements" validate:"dive"`

	// Capacity: per-node-type declared capacity. Absence of a key means
	// capacity_unknown for that node type — see the type doc comment above.
	Capacity map[string]float64 `json:"capacity,omitempty" yaml:"capacity,omitempty"`

	// Journeys is PC-124's own addition — see DeclaredJourney's own doc comment.
	Journeys []DeclaredJourney `json:"journeys,omitempty" yaml:"journeys,omitempty" validate:"dive"`

	// ServiceTimeMS is PC-124's own optional addition for PC-128 (Layer 3 degraded-
	// state latency modelling): a per-node-type latency budget, same map shape and
	// "absence means unknown, never assumed" discipline as Capacity. Not read by any
	// engine yet — PC-128's own job — added now so that ticket doesn't also need a
	// contract version bump.
	ServiceTimeMS map[string]float64 `json:"service_time_ms,omitempty" yaml:"service_time_ms,omitempty"`
}

// Validate checks this Workload against the same struct tags
// contracts/workload.schema.json is generated from.
func (w Workload) Validate() error {
	return validate.Struct(w)
}
