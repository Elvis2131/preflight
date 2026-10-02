package core

import "fmt"

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

	// AvgRequestBytes/AvgResponseBytes are PC-132's own addition — average payload
	// size per request/response, needed only for byte-based usage cost (NAT data
	// processing, data transfer, LB data-processed LCUs). Same "absence is a real,
	// stated gap" discipline as PeakRPS/SteadyRPS: never inferred, per the Card's own
	// explicit instruction ("Never infer payload sizes").
	AvgRequestBytes  *float64 `json:"avg_request_bytes,omitempty" yaml:"avg_request_bytes,omitempty"`
	AvgResponseBytes *float64 `json:"avg_response_bytes,omitempty" yaml:"avg_response_bytes,omitempty"`

	// IAMCheck is PC-135's own addition: an optional IAM authorization check applied
	// to this journey's FINAL hop only (its ultimate target — the Card's own named
	// examples, Lambda -> S3 and ECS task -> Secrets Manager, are both end-to-end
	// service calls, not per-hop network transits). Nil means this journey declares
	// no IAM check at all — ComputeJourneyFlow then evaluates it exactly as PC-125
	// left it, network-only. A per-hop IAM check would need a principal/action/
	// resource declared for every intermediate hop, real, separate, larger scope
	// this ticket does not attempt.
	IAMCheck *JourneyIAMCheck `json:"iam_check,omitempty" yaml:"iam_check,omitempty"`

	// Fallback is PC-131's own addition: an optional DECLARED degraded mode — an
	// alternative path this journey is designed to fall back to when its primary path
	// does not flow (e.g. a queue buffering payment requests while the payment rail
	// is down). Nil means no fallback was declared, and a journey that does not flow
	// simply fails: graceful degradation is never assumed (the Card's own rule).
	Fallback *JourneyFallback `json:"fallback,omitempty" yaml:"fallback,omitempty"`

	// HopPorts is PC-152's own addition: optional per-hop port overrides, keyed by the
	// hop's DESTINATION — the Path element exactly as written (a hop group such as
	// "a|b" is one key). A real multi-tier path uses different ports per hop (internet ->
	// ALB on 443, ALB -> workload on 8080, workload -> database on 5432), which a single
	// Port cannot express. A hop with no entry uses Port — never a port guessed from the
	// security group rules — so every existing workload behaves exactly as before.
	HopPorts map[string]int `json:"hop_ports,omitempty" yaml:"hop_ports,omitempty" validate:"omitempty,dive,min=1,max=65535" jsonschema:"description=Optional per-hop port overrides keyed by the hop's destination (the exact Path element). A hop with no entry uses the journey's port."`
}

// PortForHop is the port the hop whose destination is the given Path element uses: its
// HopPorts entry when declared, otherwise the journey's own Port.
func (j DeclaredJourney) PortForHop(destination string) int {
	if p, ok := j.HopPorts[destination]; ok {
		return p
	}
	return j.Port
}

// JourneyFallback is a journey's declared degraded mode (PC-131). Path uses the same
// vocabulary as DeclaredJourney.Path and is evaluated by the same flow engine against
// the same fault — a fallback only counts if it really flows under that fault.
type JourneyFallback struct {
	Description string   `json:"description" yaml:"description" validate:"required,min=1" jsonschema:"required,minLength=1,description=What the degraded mode is, e.g. requests buffered in a queue while the payment rail is down."`
	Path        []string `json:"path" yaml:"path" validate:"required,min=2,dive,required" jsonschema:"required,minItems=2"`
}

// JourneyIAMCheck names the IAM authorization query to run against a journey's final
// hop — see DeclaredJourney.IAMCheck's own doc comment for why only the final hop.
type JourneyIAMCheck struct {
	// PrincipalID is the IR node ID of the identity making the call (e.g. the
	// Lambda's own execution role) — PC-133 attaches identity policies directly to
	// identity nodes, so this is a real, resolvable IR reference, not a free-text ARN.
	PrincipalID string `json:"principal_id" yaml:"principal_id" validate:"required,min=1" jsonschema:"required,minLength=1"`
	// Action is the real AWS action string this journey's final call performs (e.g.
	// "s3:GetObject") — preserved verbatim, matched via PC-134's own wildcard-aware
	// evaluator, never guessed from the destination's NodeType.
	Action string `json:"action" yaml:"action" validate:"required,min=1" jsonschema:"required,minLength=1"`
	// ResourceARN is compared against the destination's own policy Resource/
	// NotResource elements verbatim — see IAMRequest's own doc comment (core/
	// iam_evaluate.go) for why this is a caller-supplied ARN string, not something
	// this engine can resolve from an IR node ID (core.Node has no ARN field).
	ResourceARN string `json:"resource_arn,omitempty" yaml:"resource_arn,omitempty"`
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

	// ServiceTimeSCV is PC-128's own addition (driven by the Rung 1 comparison recorded in
	// docs/PREDICTED_VS_OBSERVED.md): the squared coefficient of variation of a component's
	// service time, keyed by the SAME capacity key as Capacity. 1 is exponential service, 0 is
	// perfectly regular. Optional — absence means the latency model assumes exponential service
	// (SCV = 1, stated as an assumption), exactly as before. It is never guessed from the
	// component type: only a declared value changes the model, and then the mean uses the
	// Pollaczek-Khinchine M/G/1 formula. Measure it from the service's own latency spread.
	ServiceTimeSCV map[string]float64 `json:"service_time_scv,omitempty" yaml:"service_time_scv,omitempty" validate:"omitempty,dive,min=0" jsonschema:"description=Optional squared coefficient of variation of each component type's service time, keyed like capacity. 1 = exponential (the default assumption); 0 = perfectly regular. Declared values switch that component's mean latency to the Pollaczek-Khinchine M/G/1 formula and withhold its p95."`

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
	if err := validate.Struct(w); err != nil {
		return err
	}
	for _, j := range w.Journeys {
		if err := j.validateHopPorts(); err != nil {
			return err
		}
	}
	return nil
}

// validateHopPorts rejects a hop port that names something that is not a hop's
// destination on this journey's path (a typo would otherwise silently fall back to the
// journey port), and refuses overrides on a path that visits the same element twice,
// where a destination key would be ambiguous.
func (j DeclaredJourney) validateHopPorts() error {
	if len(j.HopPorts) == 0 {
		return nil
	}
	seen := map[string]int{}
	for _, el := range j.Path {
		seen[el]++
	}
	for key := range j.HopPorts {
		count, onPath := seen[key]
		switch {
		case !onPath || key == j.Path[0]:
			return fmt.Errorf("journey %q: hop_ports key %q is not the destination of any hop on its path", j.ID, key)
		case count > 1:
			return fmt.Errorf("journey %q: hop_ports key %q appears more than once on the path, so the override is ambiguous", j.ID, key)
		}
	}
	return nil
}
