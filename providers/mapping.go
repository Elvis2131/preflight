// This file holds the mapping SCHEMA shared by every cloud provider's own mapping data
// (aws/, azure/) — PC-22's own groundwork, extracted from providers/aws (PC-13) before
// writing a single Azure mapping: ingest/build.go consumes ResourceMapping/Registry by
// type, and if aws/ and azure/ each defined their own independent copies of these
// types, ingest would need a parallel code path per provider (or the two schemas would
// risk drifting apart, exactly the class of bug this whole codebase has repeatedly
// found and fixed once already — see core.FailoverMechanismNone's own history). One
// schema, two data sets. No provider-specific knowledge lives in this file — only the
// shape a mapping file must have, never what a specific field means for a specific
// resource type.
package providers

import (
	"embed"
	"fmt"

	"go.yaml.in/yaml/v3"

	"preflight/core"
)

// OnAbsent states what a capability field means when its source_attribute is not
// present in the user's Terraform. This is the "decided per-field, not per-resource"
// answer PC-13's Conversation asks for, expressed as a required YAML field rather than
// left to convention.
type OnAbsent string

const (
	// OnAbsentNotAssessable: no documented provider default is being asserted; the
	// capability field stays nil rather than being guessed (CLAUDE.md §5).
	OnAbsentNotAssessable OnAbsent = "not_assessable"
	// OnAbsentAssumed: a documented provider default applies; AssumedDefault and
	// AssumedCitation are both required.
	OnAbsentAssumed OnAbsent = "assumed"
)

// CapabilityMapping is one field's rule within a resource type's mapping.
type CapabilityMapping struct {
	Field           string   `yaml:"field"`
	SourceAttribute string   `yaml:"source_attribute"`
	OnAbsent        OnAbsent `yaml:"on_absent"`
	AssumedDefault  string   `yaml:"assumed_default,omitempty"`
	AssumedCitation string   `yaml:"assumed_citation,omitempty"`
}

// FailoverOutcome is the doc-verified replication/failover claim for the ENABLED state
// of a failover-driving attribute. Citation is required — the same discipline
// CapabilityMapping's on_absent=assumed case already applies: a claim this
// consequential needs an inspectable source, not a bare assertion.
type FailoverOutcome struct {
	ReplicationMode   string `yaml:"replication_mode"`
	FailoverMechanism string `yaml:"failover_mechanism"`
	Citation          string `yaml:"citation"`
}

// FailoverMapping declares how one boolean Terraform attribute determines a resource's
// replication mode and failover mechanism. Relocated here (PC-22) from
// core/internal/analyse, which held this pre-PC-22: that package is meant to hold
// provider-agnostic pure classification logic (RPOFeasibility/RTOFeasibility) only,
// but the WORDING of a failover mechanism ("Multi-AZ automatic failover to a
// synchronous standby replica") is real, provider-specific, doc-verified knowledge —
// exactly what providers/ exists to hold as data, not code. Surfaced while starting
// PC-22's Azure work, before any Azure mapping was written: dispatching by canonical
// node_type (managed_database) alone and reusing AWS's own wording for Azure SQL would
// have been a factually wrong claim about Azure, not merely an imprecise one — since
// the two providers' failover mechanisms are genuinely different (ADR-004 names this
// exact class of AWS/Azure divergence as the reason the capability model has to carry
// real per-provider detail).
//
// Only the ENABLED case carries provider-specific text (WhenEnabled). The disabled
// case is a structural fact, not a provider claim — "no failover mechanism was
// declared" is equally true regardless of provider, so it is hardcoded in
// ingest/build.go as the shared core.FailoverMechanismNone sentinel, never duplicated
// as freely-typed YAML data. This deliberately avoids reintroducing the exact bug
// PC-14 already found and fixed once (two independently-typed "none" sentinels
// silently drifting apart) — see that constant's own doc comment.
//
// Absence of SourceAttribute in the user's Terraform is always not_assessable, with no
// assumed-default path here — consistent with buildCapability's own established
// restraint (a synthesized default cannot honestly carry per-field provenance yet),
// not a new decision made just for this field.
type FailoverMapping struct {
	SourceAttribute string          `yaml:"source_attribute"`
	WhenEnabled     FailoverOutcome `yaml:"when_enabled"`
}

// EdgeMapping declares that a Terraform resource type produces no IR node of its own
// — it exists only to connect two OTHER resources (an "association" resource, e.g.
// aws_wafv2_web_acl_association). from_attribute/to_attribute name the two attributes
// whose resource-reference values become the edge's endpoints.
//
// This exists because a relationship can be expressed in Terraform as its own resource
// block rather than as an attribute of either endpoint (golden/aws/waf.tf: the
// association between aws_lb.payments and aws_wafv2_web_acl.payments) — see
// docs/IR_DESIGN_NOTE.md's WAF discussion for why this specific relationship matters:
// golden/aws-broken/waf.tf's defect 6 is a correctly-configured WAF simply never
// associated, which is only detectable if this edge exists in the IR at all.
type EdgeMapping struct {
	Type          core.EdgeType `yaml:"type"`
	FromAttribute string        `yaml:"from_attribute"`
	ToAttribute   string        `yaml:"to_attribute"`
}

// ResourceMapping is one YAML file's full content: either a node mapping (NodeType +
// Capabilities set, Edge unset) or an edge-only mapping (Edge set, NodeType/
// Capabilities unset) — never both, never neither.
type ResourceMapping struct {
	ResourceType string              `yaml:"resource_type"`
	NodeType     core.NodeType       `yaml:"node_type,omitempty"`
	Capabilities []CapabilityMapping `yaml:"capabilities,omitempty"`
	Failover     *FailoverMapping    `yaml:"failover,omitempty"`
	Edge         *EdgeMapping        `yaml:"edge,omitempty"`

	// ReferenceEdgeType, when set, is the edge type ingest must use for any OTHER
	// resource's reference TO a node of this type — e.g. a reference to a subnet is
	// containment (contained_in), not an ordinary dependency (depends_on). Unset means
	// depends_on, the long-standing default for the original 8 golden types (PC-78:
	// added for network-substrate types where PRD §4's contained_in edge is the
	// semantically correct one, not a new general mechanism this ticket invented — the
	// edge type itself was already frozen by PC-7).
	ReferenceEdgeType core.EdgeType `yaml:"reference_edge_type,omitempty"`

	// IngestOnly marks a node mapping that HCL ingest understands but the canvas cannot
	// author yet (PC-149: aws_default_network_acl — the canvas has no way to mark a NACL
	// as the VPC default). The service catalog omits it, so the palette never offers a
	// service whose semantics the canvas would silently drop.
	IngestOnly bool `yaml:"ingest_only,omitempty"`

	// NetworkRole names a structural role core needs to recognise without knowing a
	// provider's resource names (PC-151): "subnet", "vpc", "route_table", "elastic_ip" or "web_acl". Ingest stamps it on the node
	// as RawAttributes["network_role"]. Before this, core could only identify a subnet by
	// its having an explicit route table association, so a subnet relying on the VPC's
	// implicit main route table was not recognised as a subnet at all.
	NetworkRole string `yaml:"network_role,omitempty"`

	// DefaultNACL marks a NACL mapping whose node is its VPC's DEFAULT network ACL (PC-153: the
	// canvas has no resource-type name to key on, so the mapping says it). Ingest stamps
	// RawAttributes["default_nacl"] = true on such a node, the same attribute HCL ingest sets for
	// aws_default_network_acl, so core resolves both producers identically.
	DefaultNACL bool `yaml:"default_nacl,omitempty"`

	// PublicAddressModel says HOW a resource of this type comes to hold a public IPv4 address,
	// so core can answer "can it use an internet gateway" without naming a provider (PC-156).
	// "launch_attribute" means the address is assigned at launch: a launch-time setting on the
	// resource overrides the subnet's own auto-assign attribute, and an Elastic IP association
	// gives it one regardless. Unset means the service's public-address behaviour is not
	// modelled, and a journey from it through an internet gateway is not_assessable.
	PublicAddressModel string `yaml:"public_address_model,omitempty"`

	// TrackUnresolved lists attribute names whose presence-but-not-literal state must be
	// recorded on the node (RawAttributes["unresolved_attributes"]). A non-literal attribute is
	// otherwise indistinguishable from an absent one, and absence means a documented default
	// while a variable means "unknown" (I4).
	TrackUnresolved []string `yaml:"track_unresolved,omitempty"`

	// CapabilityLevel is PC-107's own registry entry: how much of this specific AWS
	// service Preflight can actually simulate, on core.CapabilityLevel's 9-rung
	// ladder. Required for every node mapping (validated against
	// core.MaxImplementedCapabilityLevel(NodeType) at Load() time — a mapping cannot
	// claim more than its structural NodeType's real engines reach); forbidden for an
	// edge mapping, which produces no node to have a capability level at all.
	CapabilityLevel core.CapabilityLevel `yaml:"capability_level,omitempty"`
}

// IsEdgeMapping reports whether this mapping produces an edge rather than a node.
func (m ResourceMapping) IsEdgeMapping() bool { return m.Edge != nil }

// validate enforces PC-13's acceptance criterion structurally: every capability field
// in every mapping carries an explicit provenance tag (OnAbsent), and an "assumed"
// field is not permitted to omit the default/citation that make it checkable rather
// than a bare assertion. Provider-agnostic: identical rules apply to every provider's
// mapping data, which is the whole point of sharing this validator rather than each
// provider re-implementing (and potentially loosening) it independently.
func (m ResourceMapping) validate() error {
	if m.ResourceType == "" {
		return fmt.Errorf("mapping is missing resource_type")
	}

	if m.Edge != nil {
		if m.NodeType != "" || len(m.Capabilities) > 0 {
			return fmt.Errorf("mapping %s: edge and node_type/capabilities are mutually exclusive — this resource type produces one or the other, never both", m.ResourceType)
		}
		if m.Edge.Type == "" {
			return fmt.Errorf("mapping %s: edge.type is required", m.ResourceType)
		}
		if m.Edge.FromAttribute == "" || m.Edge.ToAttribute == "" {
			return fmt.Errorf("mapping %s: edge.from_attribute and edge.to_attribute are both required", m.ResourceType)
		}
		if m.CapabilityLevel != "" {
			return fmt.Errorf("mapping %s: capability_level is not permitted on an edge mapping — an edge produces no node to have a capability level", m.ResourceType)
		}
		return nil
	}

	if m.NodeType == "" {
		return fmt.Errorf("mapping %s is missing node_type (and is not an edge mapping)", m.ResourceType)
	}
	if m.CapabilityLevel == "" {
		return fmt.Errorf("mapping %s: capability_level is required (PC-107)", m.ResourceType)
	}
	if !m.CapabilityLevel.Valid() {
		return fmt.Errorf("mapping %s: capability_level %q is not one of core.CapabilityLevel's 9 defined rungs", m.ResourceType, m.CapabilityLevel)
	}
	if ceiling := core.MaxImplementedCapabilityLevel(m.NodeType); !ceiling.AtLeast(m.CapabilityLevel) {
		return fmt.Errorf("mapping %s: capability_level %q exceeds the real ceiling %q for node_type %q (core.MaxImplementedCapabilityLevel) — no engine in this codebase implements that much behaviour for this structural type yet", m.ResourceType, m.CapabilityLevel, ceiling, m.NodeType)
	}
	if m.NetworkRole != "" && m.NetworkRole != "subnet" && m.NetworkRole != "vpc" && m.NetworkRole != "route_table" && m.NetworkRole != "elastic_ip" && m.NetworkRole != "web_acl" {
		return fmt.Errorf("mapping %s: network_role %q must be \"subnet\", \"vpc\", \"route_table\", \"elastic_ip\" or \"web_acl\"", m.ResourceType, m.NetworkRole)
	}
	if m.DefaultNACL && m.NodeType != core.NodeTypeNetworkBoundary {
		return fmt.Errorf("mapping %s: default_nacl is only meaningful on a network_boundary node", m.ResourceType)
	}
	if m.PublicAddressModel != "" && m.PublicAddressModel != "launch_attribute" {
		return fmt.Errorf("mapping %s: public_address_model %q must be \"launch_attribute\"", m.ResourceType, m.PublicAddressModel)
	}
	if m.ReferenceEdgeType != "" {
		switch m.ReferenceEdgeType {
		case core.EdgeTypeDependsOn, core.EdgeTypeRoutesTo, core.EdgeTypeReadsWrites,
			core.EdgeTypeAuthenticatesVia, core.EdgeTypeReplicatesTo, core.EdgeTypeContainedIn:
			// valid
		default:
			return fmt.Errorf("mapping %s: reference_edge_type %q is not one of PRD §4's frozen edge types", m.ResourceType, m.ReferenceEdgeType)
		}
	}
	for _, c := range m.Capabilities {
		if c.Field == "" {
			return fmt.Errorf("mapping %s: a capability is missing field", m.ResourceType)
		}
		switch c.OnAbsent {
		case OnAbsentNotAssessable:
			// nothing further required
		case OnAbsentAssumed:
			if c.AssumedDefault == "" || c.AssumedCitation == "" {
				return fmt.Errorf("mapping %s field %s: on_absent=assumed requires both assumed_default and assumed_citation", m.ResourceType, c.Field)
			}
		default:
			return fmt.Errorf("mapping %s field %s: on_absent must be %q or %q, got %q",
				m.ResourceType, c.Field, OnAbsentNotAssessable, OnAbsentAssumed, c.OnAbsent)
		}
	}
	if m.Failover != nil {
		if m.Failover.SourceAttribute == "" {
			return fmt.Errorf("mapping %s: failover.source_attribute is required", m.ResourceType)
		}
		we := m.Failover.WhenEnabled
		if we.ReplicationMode == "" || we.FailoverMechanism == "" || we.Citation == "" {
			return fmt.Errorf("mapping %s: failover.when_enabled requires replication_mode, failover_mechanism, and citation all set", m.ResourceType)
		}
	}
	return nil
}

// Registry is a loaded, validated set of resource mappings keyed by that provider's own
// Terraform resource type name (e.g. "aws_lb", "azurerm_application_gateway"). One
// Registry only ever holds one provider's mappings — aws.Load() and azure.Load() each
// return their own — never a merged multi-provider set, since a single Terraform
// bundle is unambiguously one provider's resource-type vocabulary.
type Registry map[string]ResourceMapping

// Load reads and validates every *.yaml file in fs (a provider package's own embedded
// mapping directory) into a Registry. Called once per provider, typically at process
// start — there is no dynamic reloading, consistent with these being
// frozen-per-release mapping data, not live configuration. providerLabel is used only
// for error messages (e.g. "providers/aws", "providers/azure").
func Load(fs embed.FS, providerLabel string) (Registry, error) {
	entries, err := fs.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("%s: read embedded dir: %w", providerLabel, err)
	}

	reg := Registry{}
	for _, e := range entries {
		data, err := fs.ReadFile(e.Name())
		if err != nil {
			return nil, fmt.Errorf("%s: read %s: %w", providerLabel, e.Name(), err)
		}

		var m ResourceMapping
		if err := yaml.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("%s: parse %s: %w", providerLabel, e.Name(), err)
		}
		if err := m.validate(); err != nil {
			return nil, fmt.Errorf("%s: %s: %w", providerLabel, e.Name(), err)
		}
		if _, dup := reg[m.ResourceType]; dup {
			return nil, fmt.Errorf("%s: duplicate resource_type %q (in %s)", providerLabel, m.ResourceType, e.Name())
		}
		reg[m.ResourceType] = m
	}
	return reg, nil
}

// Merge combines multiple providers' Registries into one, keyed by resource_type as
// always. Safe and unambiguous by construction: every provider's own Terraform
// resource type names are already namespaced by that provider's own prefix
// convention (aws_*, azurerm_*, ...) — there is no real collision risk merging
// registries from different providers, only from loading the SAME provider's mapping
// data twice (which duplicate() rejects at Load() time, per-provider, already).
// PC-29: this is what lets ingest.Ingest (and callers like server.Assess) work against
// a bundle without knowing in advance which cloud authored it.
func Merge(registries ...Registry) Registry {
	merged := Registry{}
	for _, r := range registries {
		for resourceType, mapping := range r {
			merged[resourceType] = mapping
		}
	}
	return merged
}

// Lookup returns the mapping for a Terraform resource type, and whether one exists. A
// resource type with no mapping is not an error at this layer — CLAUDE.md §14/§27: a
// resource outside the golden vocabulary surfaces as not_assessable upstream (ingest's
// job), never silently ignored, but it is also never a reason for this registry itself
// to fail.
func (r Registry) Lookup(resourceType string) (ResourceMapping, bool) {
	m, ok := r[resourceType]
	return m, ok
}
