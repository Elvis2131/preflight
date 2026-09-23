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
}

// Validate checks this Workload against the same struct tags
// contracts/workload.schema.json is generated from.
func (w Workload) Validate() error {
	return validate.Struct(w)
}
