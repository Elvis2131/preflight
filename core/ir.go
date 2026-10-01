package core

// NodeType is the canonical, cloud-agnostic node vocabulary (PRD §4): "Provider
// resources normalise to capability-level node types." Exactly the eleven listed —
// adding a twelfth requires the scenario-driven minimalism test (CLAUDE.md §8): which
// of the six golden scenarios needs it?
type NodeType string

const (
	NodeTypeCompute            NodeType = "compute"
	NodeTypeContainerWorkload  NodeType = "container_workload"
	NodeTypeManagedDatabase    NodeType = "managed_database"
	NodeTypeCache              NodeType = "cache"
	NodeTypeLoadBalancer       NodeType = "load_balancer"
	NodeTypeQueueStream        NodeType = "queue/stream"
	NodeTypeObjectStore        NodeType = "object_store"
	NodeTypeDNS                NodeType = "dns"
	NodeTypeNetworkBoundary    NodeType = "network_boundary"
	NodeTypeIdentity           NodeType = "identity"
	NodeTypeExternalDependency NodeType = "external_dependency"
)

// EdgeType is the canonical edge vocabulary (PRD §4), verbatim.
type EdgeType string

const (
	EdgeTypeDependsOn        EdgeType = "depends_on"
	EdgeTypeRoutesTo         EdgeType = "routes_to"
	EdgeTypeReadsWrites      EdgeType = "reads/writes"
	EdgeTypeAuthenticatesVia EdgeType = "authenticates_via"
	EdgeTypeReplicatesTo     EdgeType = "replicates_to"
	EdgeTypeContainedIn      EdgeType = "contained_in"
)

// ResolutionState is PRD §4's tri-state: "partial input is the normal case." Unresolved
// elements propagate to analysis as not_assessable (I4), never a passing or failing
// result — this is the IR-level half of that guarantee; core/analyse's obligation to
// actually honor it is a separate, later concern (PC-14).
type ResolutionState string

const (
	// ResolutionKnown: fully declared in the source bundle.
	ResolutionKnown ResolutionState = "known"
	// ResolutionInferred: resolved from a reference the engine could follow.
	ResolutionInferred ResolutionState = "inferred"
	// ResolutionUnresolved: references a declaration not present in the bundle.
	ResolutionUnresolved ResolutionState = "unresolved"
)

// CapabilityModel is PRD §4's provider capability model: "typed, per-node capabilities
// that failure and compliance semantics actually depend on... Structured capabilities,
// not a raw-attribute bag; raw provider attributes are additionally retained for
// attribute-level compliance checks" (see Node.RawAttributes).
//
// Every field is a pointer: a capability model is attached to a node whose resolution
// may be inferred or unresolved, in which case some or all capabilities are simply not
// knowable yet — a nil field is not a claim of "false" or "none", it is the absence of
// an answer (I4 in miniature, at the field level).
type CapabilityModel struct {
	ReplicationMode       *string `json:"replication_mode,omitempty" jsonschema:"description=e.g. sync or async"`
	FailoverMechanism     *string `json:"failover_mechanism,omitempty"`
	FailoverTimeClass     *string `json:"failover_time_class,omitempty" jsonschema:"description=A documented class (e.g. seconds/minutes), not a precise measured duration — a precise number is an I5 resilience-tier claim, only assertable from Rung-3 evidence."`
	BackupSemantics       *string `json:"backup_semantics,omitempty"`
	EncryptionMechanism   *string `json:"encryption_mechanism,omitempty"`
	KeyOwnership          *string `json:"key_ownership,omitempty" jsonschema:"description=e.g. aws-managed or customer-managed (CMK)"`
	MultiAZImplementation *string `json:"multi_az_implementation,omitempty"`
	MaintenanceBehaviour  *string `json:"maintenance_behaviour,omitempty"`
}

// Sizing is PC-115's own, justified, recorded extension: the IR deliberately never
// carried sizing before this (scenario-driven minimalism — no failure scenario needed
// it), but the cost engine (PC-116/117) does, so this is added now rather than
// invented as scope creep. Every field is a pointer, same discipline as
// CapabilityModel: absence is "not known", never a default — PC-115's own acceptance
// criterion, "missing sizing yields cost_unknown, never a default."
//
// Count is a sizing fact (e.g. a desired task/instance count as declared in
// Terraform) and must never be read as a CAPACITY fact — Workload.Capacity (PRD §4's
// own capacity semantics) is the only source of truth for capacity, a structurally
// separate, workload-declared map; nothing in this codebase's capacity/simulate code
// reads Sizing at all (proven by core/sizing_test.go's own structural check).
//
// One flat struct across every costable component type, not one struct per NodeType:
// each field belongs to a different subset of node types (InstanceType to
// compute/container_workload, InstanceClass to managed_database, CacheNodeType to
// cache, LoadBalancerType to load_balancer) and simply stays nil for types it doesn't
// apply to — the same "one shape, many optional fields" discipline CapabilityModel
// already uses, rather than a NodeType-keyed union this codebase has no precedent for.
//
// Region is deliberately NOT a field here: Workload.Regions already declares it
// (core/workload.go) — this ticket's own Card says as much ("region: already present
// on containers; reuse"), though the accurate description is "present on the
// workload," not on any per-node container; no per-node region attribute exists
// anywhere in the IR (core/simulate.go's own region_loss doc comment already states
// this directly). Recorded here as a correction rather than silently reproducing the
// Card's imprecise wording.
type Sizing struct {
	InstanceType       *string `json:"instance_type,omitempty" jsonschema:"description=e.g. an EC2 instance type (compute) — mutually relevant with TaskCPU/TaskMemory, never both populated for the same node."`
	TaskCPU            *string `json:"task_cpu,omitempty" jsonschema:"description=Container task vCPU allocation (container_workload) — e.g. ECS task_definition cpu."`
	TaskMemory         *string `json:"task_memory,omitempty" jsonschema:"description=Container task memory allocation (container_workload) — e.g. ECS task_definition memory."`
	Count              *int    `json:"count,omitempty" jsonschema:"description=A declared instance/task/node count — a SIZING fact only, never a capacity fact (PRD capacity semantics; Workload.Capacity is the only capacity source of truth)."`
	InstanceClass      *string `json:"instance_class,omitempty" jsonschema:"description=Managed database instance class (managed_database) — e.g. RDS instance_class. Multi-AZ is NOT duplicated here — see CapabilityModel.MultiAZImplementation."`
	AllocatedStorageGB *int    `json:"allocated_storage_gb,omitempty" jsonschema:"description=Allocated storage, in GB (managed_database)."`
	StorageType        *string `json:"storage_type,omitempty" jsonschema:"description=Storage type (managed_database) — e.g. RDS storage_type (gp3, io1, ...)."`
	CacheNodeType      *string `json:"cache_node_type,omitempty" jsonschema:"description=Cache node type (cache) — e.g. ElastiCache node_type."`
	LoadBalancerType   *string `json:"load_balancer_type,omitempty" jsonschema:"description=Load balancer type (load_balancer) — ALB or NLB. Usage-based charges (data processed) are not_assessable until traffic inputs exist (PC-124)."`
	// Region is PC-110's addition: the AWS region code (e.g. eu-west-1) this component runs
	// in, stated by the architect. First step of the per-component region resolution
	// (core.ResolveComponentRegion); absent means the workload's single region is used if
	// there is exactly one, otherwise the component is unpriced until a region is chosen.
	Region *string `json:"region,omitempty" jsonschema:"description=AWS region code this component runs in (e.g. eu-west-1). Absent: the workload's region is used only if it declares exactly one; otherwise cost is cost_unknown naming region. Never defaulted."`
}

// Node is one element of the IR's canonical semantic model, carrying its provider
// capability model alongside it (the two-level model, PRD §4).
//
// V1 SIMPLIFICATION, stated rather than hidden: PRD §4 says "every field in every
// response carries a source tag" — the fully general form is per-field provenance
// (each of a node's individual attributes independently tagged). This v1 struct
// attaches a single Provenance to the whole node instead. Per-field provenance is a
// real design question PC-11 owns; modeling it here would be inventing PC-11's answer
// rather than freezing what PRD §4 and Design §5 actually specify at this level of
// detail. Do not treat this simplification as a decision that IR nodes never need
// per-field provenance — it is an open item, not a resolved one.
type Node struct {
	ID         string           `json:"id" validate:"required" jsonschema:"required,minLength=1"`
	Type       NodeType         `json:"type" validate:"required,oneof=compute container_workload managed_database cache load_balancer queue/stream object_store dns network_boundary identity external_dependency" jsonschema:"required"`
	Resolution ResolutionState  `json:"resolution" validate:"required,oneof=known inferred unresolved" jsonschema:"required,enum=known,enum=inferred,enum=unresolved"`
	Capability *CapabilityModel `json:"capability,omitempty" jsonschema:"description=Present only when Resolution is known or inferred enough to populate it; a node with capability entirely nil is not a claim of no capabilities, only that none are known yet."`

	// Sizing is PC-115's addition — see Sizing's own doc comment. Nil (not a
	// zero-valued struct) means no sizing is known for this node at all, distinct
	// from a Sizing struct whose individual fields are each independently nil.
	Sizing *Sizing `json:"sizing,omitempty" jsonschema:"description=Present only when at least one sizing fact is known for this node; absent entirely means cost_unknown for every costable dimension, never a default."`

	// IAMIdentityPolicies/IAMTrustPolicy/IAMResourcePolicy are PC-133's addition — see
	// core/iam.go's own doc comment for the full modelled-scope statement.
	// IAMIdentityPolicies is only ever populated on an identity node (a role) — its
	// own inline and managed policies. IAMTrustPolicy is only ever populated on an
	// identity node — who may assume it. IAMResourcePolicy is only ever populated on
	// the RESOURCE a policy is declared on directly (e.g. an S3 bucket policy),
	// never an identity node — the two are mutually exclusive by construction (no
	// node in this codebase is both an identity and a resource-policy target).
	IAMIdentityPolicies []PolicyDocument `json:"iam_identity_policies,omitempty" validate:"dive"`
	IAMTrustPolicy      *PolicyDocument  `json:"iam_trust_policy,omitempty"`
	IAMResourcePolicy   *PolicyDocument  `json:"iam_resource_policy,omitempty"`

	// RawAttributes retains provider-specific attributes verbatim (PRD §4: "raw
	// provider attributes are additionally retained for attribute-level compliance
	// checks") — e.g. an AWS-specific field the canonical CapabilityModel has no slot
	// for yet, but a compliance rule (PC-18) still needs to inspect directly.
	RawAttributes map[string]any `json:"raw_attributes,omitempty"`

	Provenance Provenance `json:"provenance" validate:"required" jsonschema:"required"`
}

// Edge is one relationship in the IR's canonical semantic model.
type Edge struct {
	ID         string          `json:"id" validate:"required" jsonschema:"required,minLength=1"`
	Type       EdgeType        `json:"type" validate:"required,oneof=depends_on routes_to reads/writes authenticates_via replicates_to contained_in" jsonschema:"required"`
	From       string          `json:"from" validate:"required" jsonschema:"required,description=Node ID."`
	To         string          `json:"to" validate:"required" jsonschema:"required,description=Node ID."`
	Resolution ResolutionState `json:"resolution" validate:"required,oneof=known inferred unresolved" jsonschema:"required"`
	Provenance Provenance      `json:"provenance" validate:"required" jsonschema:"required"`

	// RawAttributes is PC-111's ir.schema.json 1.1.0 addition — the Edge-level
	// counterpart to Node.RawAttributes above, added for the same reason: a routes_to
	// edge needs to carry data a plain (From, To, Type) triple cannot express at all —
	// specifically a route's destination CIDR (core/internal/analyse's longest-prefix-
	// match route selection needs the real CIDR, not just "a route exists to this
	// target"). Optional and additive: every edge type that existed before this ticket
	// (contained_in, depends_on, etc.) is unaffected and simply carries nil here.
	RawAttributes map[string]any `json:"raw_attributes,omitempty"`
}

// IR is the top-level architecture model (PRD §4). "The model is versioned. Every
// server call against a session produces Version N; diffs between versions are
// first-class." VersionNumber is the sequential N; VersionHash is Design §5's "version
// hash" — a content hash of this document, so two IR documents can be compared for
// identity without depending on VersionNumber bookkeeping alone.
type IR struct {
	SchemaVersion string `json:"schema_version" validate:"required" jsonschema:"required,description=Contract version of this schema itself — see CHANGELOG.md. Not the same thing as VersionNumber below, which versions a document instance, not the contract."`

	VersionNumber int    `json:"version_number" validate:"gte=1" jsonschema:"required,minimum=1"`
	VersionHash   string `json:"version_hash" validate:"required" jsonschema:"required,minLength=1"`

	Nodes []Node `json:"nodes" validate:"dive"`
	Edges []Edge `json:"edges" validate:"dive"`
}

// Validate checks this IR against the same struct tags contracts/ir.schema.json is
// generated from.
func (d IR) Validate() error {
	return validate.Struct(d)
}
