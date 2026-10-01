package core

// CanvasDocument is the wire contract a canvas UI posts to become an assessable
// architecture (PC-86) — the SIXTH frozen contract, given the same treatment as the
// five PC-7 already froze (IR, provenance, workload, finding, ADR): generated into
// contracts/canvas.schema.json from this exact struct, versioned independently per
// Design §5.
//
// This is a deliberate decision, not deferred to "whatever serialize.ts happens to
// emit": canvas/src/types.ts's own CanvasDocument is now the MIRROR of this struct
// (the same relationship core/ir.go's NodeType/EdgeType already has with
// canvas/src/goldenVocabulary.ts — one source of truth, mirrored deliberately, not
// redefined) rather than the reverse. Without this, a future ticket (PC-87 adding
// capability structure, for instance) could change what the canvas emits with
// nothing on this side noticing until a real request failed oddly — exactly the
// class of silent producer/consumer drift this codebase has already found and fixed
// twice (the FailoverMechanismNone sentinel mismatch, PC-14; the BuildScorecard/
// ComputeDelta not_assessable exclusion, PC-83). A frozen, tested contract is the
// cheaper fix, paid once, rather than a repeat of that same class of bug at a third
// boundary.
type CanvasNode struct {
	ID    string   `json:"id" validate:"required" jsonschema:"required,minLength=1"`
	Type  NodeType `json:"type" validate:"required,oneof=compute container_workload managed_database cache load_balancer queue/stream object_store dns network_boundary identity external_dependency" jsonschema:"required"`
	Label string   `json:"label" validate:"required" jsonschema:"required,minLength=1"`

	// Capability: free-form string key/value pairs entered via the canvas UI.
	// Deliberately untyped at this contract's current version — PC-87 (the NFR form)
	// and a later capability-editing ticket own giving these real, per-node-type
	// structure and validation against workload.schema.json's own semantics. A
	// future version of THIS contract (canvas.schema.json bumped, per
	// contracts/CHANGELOG.md's own convention) is where that structure would land,
	// not a silent, undocumented shape change.
	Capability map[string]string `json:"capability" jsonschema:"description=Free-form capability key/value pairs entered via the canvas UI. Untyped in this contract version — see CanvasNode's own doc comment."`

	// Sizing is PC-110's own addition — the Design inspector's sizing fields
	// (instance type/class, count, storage), same free-form string-map shape as
	// Capability above and for the same reason: this contract's current version
	// keeps canvas-entered values untyped, with real structure/validation living in
	// ingest/canvas.go's own translation into core.Sizing (PC-115's typed IR field).
	// Keys are core.Sizing's own canonical field names (instance_type, count,
	// instance_class, allocated_storage_gb, storage_type, cache_node_type,
	// load_balancer_type, task_cpu, task_memory) — never a provider-specific
	// Terraform attribute name, the same "the palette IS the golden vocabulary"
	// reasoning CanvasNode.Type itself already follows. A blank/absent key means
	// that sizing fact is unknown for this node, never a guessed default — the Cost
	// Engine (PC-117) reports cost_unknown for it, exactly as it does for a
	// Terraform-ingested node missing the same fact.
	Sizing map[string]string `json:"sizing,omitempty" jsonschema:"description=Free-form sizing key/value pairs entered via the canvas UI Design inspector (PC-110). Keys are core.Sizing's own canonical field names. Blank/absent means unknown, never defaulted — the Cost Engine reports cost_unknown for that dimension."`

	// ServiceID is PC-136's own addition: the real provider Terraform resource_type
	// string (e.g. "aws_db_instance", "aws_lambda_function") the architect selected
	// for this node — the SAME key providers.Registry is keyed by for the Terraform
	// ingest path (providers/mapping.go's own ResourceMapping.ResourceType). Required
	// because CanvasNode.Type alone is a structural category, not a specific
	// service — "compute" cannot distinguish EC2 from Lambda, which carry different
	// PC-107 capability levels. Absent or a value with no registry entry (or one whose
	// own NodeType disagrees with this node's declared Type) leaves capability_level
	// unresolved, exactly as an out-of-vocabulary Terraform resource would — never
	// guessed, never defaulted (I4).
	ServiceID string `json:"service_id,omitempty" jsonschema:"description=The real provider resource_type this node represents (e.g. aws_db_instance) — the same key providers.Registry uses for the Terraform ingest path. Absent or unresolvable leaves capability_level unresolved, never guessed."`

	// SecurityGroupRules is PC-137's own addition: real, structured Security Group
	// rules the architect authored on this node — mirrors ingest/securitygroups.go's
	// own normalized rule shape exactly (direction, protocol, from_port/to_port,
	// cidr_blocks, source_security_group), the same shape core.SecurityGroupProfile
	// already reads back out of RawAttributes["security_group_rules"] for a
	// Terraform-ingested SG. A structured list, not a string map like Capability/
	// Sizing above — a rule is not a flat key/value pair, it is itself a small
	// record (PC-112's own SGRule shape, unavoidably plural: a security group is
	// exactly a LIST of these). A component is attached to a security group the
	// same way Terraform ingest already represents it: a depends_on edge from the
	// component to this node — no new edge type, no new attachment mechanism.
	SecurityGroupRules []CanvasSecurityGroupRule `json:"security_group_rules,omitempty" jsonschema:"description=Security Group rules authored on this node (PC-137) — mirrors ingest/securitygroups.go's own normalized rule shape. A component attaches to this security group via a depends_on edge exactly as the Terraform ingest path already represents SG attachment."`

	// AvailabilityZone and CIDRBlock are PC-105's own addition — real placement
	// facts for the VPC/subnet containers. They are stamped onto RawAttributes under
	// the SAME keys ("availability_zone", "cidr_block") the Terraform ingest path
	// already uses for aws_subnet/aws_vpc, so every downstream reader (zone-kill
	// blast radius, cross-AZ cost, NACL/SG CIDR matching) sees one IR shape from
	// both producers. The Region ⊃ AZ nesting is NOT a node: in the IR an AZ is an
	// attribute of a subnet (exactly as in Terraform), and a Region is
	// Workload.Regions — so the canvas's visual AZ/Region grouping writes these
	// attributes instead of inventing nodes the Terraform path has no equivalent for.
	// Absent means unknown, never defaulted (I4).
	AvailabilityZone string `json:"availability_zone,omitempty" jsonschema:"description=The Availability Zone a subnet resides in (e.g. eu-west-1a) — one value, since a subnet cannot span zones. Absent means unknown."`
	CIDRBlock        string `json:"cidr_block,omitempty" jsonschema:"description=IPv4 CIDR block of a VPC or subnet. Absent means unknown."`
}

// CanvasSecurityGroupRule is one Security Group rule authored on the canvas — the
// wire-shape counterpart to ingest/securitygroups.go's own normalizeSGRule output
// map, typed here instead of left as map[string]any because this is
// architect-entered, user-facing input (unlike RawAttributes' own internal carrier
// role), where real validation (a direction that IS "ingress"/"egress", a required
// protocol) is worth having at the contract boundary.
type CanvasSecurityGroupRule struct {
	Direction string `json:"direction" validate:"required,oneof=ingress egress" jsonschema:"required,enum=ingress,enum=egress"`
	Protocol  string `json:"protocol" validate:"required" jsonschema:"required,description=tcp / udp / icmp / -1 (AWS's own all-protocols sentinel)."`
	FromPort  int    `json:"from_port,omitempty"`
	ToPort    int    `json:"to_port,omitempty"`

	// CIDRBlocks and SourceSecurityGroup are mutually exclusive rule-source shapes,
	// exactly as AWS's own real Security Group rules are (a CIDR-sourced rule or an
	// SG-referencing rule, never both) — mirrors core.SGRule's own CIDRs/SourceSG
	// split. SourceSecurityGroup is another canvas node's own ID (a plain string
	// match against SG node IDs, the same mechanism a Terraform-ingested
	// aws_security_group_rule's source_security_group_id reference already reduces
	// to by the time it reaches core.SGRule.SourceSG).
	CIDRBlocks          []string `json:"cidr_blocks,omitempty"`
	SourceSecurityGroup string   `json:"source_security_group,omitempty"`
}

type CanvasEdge struct {
	ID   string   `json:"id" validate:"required" jsonschema:"required,minLength=1"`
	Type EdgeType `json:"type" validate:"required,oneof=depends_on routes_to reads/writes authenticates_via replicates_to contained_in" jsonschema:"required"`
	From string   `json:"from" validate:"required" jsonschema:"required,minLength=1"`
	To   string   `json:"to" validate:"required" jsonschema:"required,minLength=1"`
}

type CanvasDocument struct {
	Nodes []CanvasNode `json:"nodes" validate:"dive" jsonschema:"required"`
	Edges []CanvasEdge `json:"edges" validate:"dive" jsonschema:"required"`
}

// Validate checks this CanvasDocument against the same struct tags
// contracts/canvas.schema.json is generated from.
func (d CanvasDocument) Validate() error {
	return validate.Struct(d)
}
