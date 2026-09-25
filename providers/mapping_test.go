package providers

import (
	"testing"

	"preflight/core"
)

// TestValidate_RejectsCapabilityFieldWithNoOnAbsent is PC-13's second acceptance
// criterion, relocated here (PC-22) when validate() moved from providers/aws into
// this shared package — it tests the schema's own validation rule, not anything
// AWS-specific, so it belongs where that rule is actually declared.
func TestValidate_RejectsCapabilityFieldWithNoOnAbsent(t *testing.T) {
	invalid := ResourceMapping{
		ResourceType: "test_widget",
		NodeType:     "compute",
		Capabilities: []CapabilityMapping{
			{Field: "some_field", SourceAttribute: "some_attr"}, // OnAbsent left zero-value
		},
	}
	if err := invalid.validate(); err == nil {
		t.Fatal("expected validate() to reject a capability field with no on_absent tag")
	}
}

// TestValidate_AcceptsA9thResourceTypeWithZeroSpecialCasing is PC-13's third
// acceptance criterion, relocated here for the same reason as above: the schema
// itself must accept any provider's Nth resource type shaped like the first 8,
// without any special-casing keyed to a specific resource type. The structural half
// of this criterion (core/internal/analyse imports no providers package at all)
// stays in providers/aws/mapping_test.go, since it inspects analyse's own source, not
// this schema.
func TestValidate_AcceptsA9thResourceTypeWithZeroSpecialCasing(t *testing.T) {
	ninth := ResourceMapping{
		ResourceType: "test_widget",
		NodeType:     "compute",
		Capabilities: []CapabilityMapping{
			{Field: "widget_size", SourceAttribute: "size", OnAbsent: OnAbsentNotAssessable},
		},
		CapabilityLevel: core.CapabilityConfiguration,
	}
	if err := ninth.validate(); err != nil {
		t.Fatalf("a 9th mapping, shaped exactly like any real one, failed validation: %v", err)
	}
	reg := Registry{ninth.ResourceType: ninth}
	if _, ok := reg.Lookup("test_widget"); !ok {
		t.Fatal("Lookup failed for a dynamically-added 9th mapping")
	}
}

// TestValidate_RejectsCapabilityLevelAboveTheRealCeiling is PC-107's own negative
// control: a mapping claiming more than core.MaxImplementedCapabilityLevel actually
// implements for its node_type must be refused at validate() time, not silently
// accepted — proving the ceiling enforcement in mapping.go's validate() is real, not
// vacuous.
func TestValidate_RejectsCapabilityLevelAboveTheRealCeiling(t *testing.T) {
	// "compute"'s own ceiling is core.CapabilityConfiguration (no dedicated engine
	// exists for it yet) — claiming FAILURE_SIMULATION must be refused.
	overclaiming := ResourceMapping{
		ResourceType:    "test_widget",
		NodeType:        "compute",
		CapabilityLevel: core.CapabilityFailureSimulation,
	}
	if err := overclaiming.validate(); err == nil {
		t.Fatal("expected validate() to reject a capability_level above the real ceiling for its node_type")
	}
}

// TestValidate_RejectsMissingCapabilityLevel is PC-107's own first acceptance
// criterion in miniature: every node mapping must declare a capability_level.
func TestValidate_RejectsMissingCapabilityLevel(t *testing.T) {
	missing := ResourceMapping{ResourceType: "test_widget", NodeType: "compute"}
	if err := missing.validate(); err == nil {
		t.Fatal("expected validate() to reject a node mapping with no capability_level")
	}
}

// TestValidate_RejectsCapabilityLevelOnAnEdgeMapping proves the mutual-exclusion the
// other direction: an edge mapping (which produces no node) must not declare one.
func TestValidate_RejectsCapabilityLevelOnAnEdgeMapping(t *testing.T) {
	edge := ResourceMapping{
		ResourceType:    "test_widget_assoc",
		Edge:            &EdgeMapping{Type: core.EdgeTypeDependsOn, FromAttribute: "a", ToAttribute: "b"},
		CapabilityLevel: core.CapabilityConfiguration,
	}
	if err := edge.validate(); err == nil {
		t.Fatal("expected validate() to reject a capability_level on an edge mapping")
	}
}
