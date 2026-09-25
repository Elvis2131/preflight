package aws

// PC-107: proves the real, loaded AWS registry's own capability_level entries are
// honest — every node mapping has one, it's a valid rung, and it never exceeds
// core.MaxImplementedCapabilityLevel for its own node_type. Load()'s own validate()
// already enforces this at load time (see the negative-control proof in
// providers/mapping_test.go); this test proves the POSITIVE half — the real data
// actually passes today.

import (
	"testing"

	"preflight/core"
)

func TestAWSRegistry_EveryNodeMappingHasAnHonestCapabilityLevel(t *testing.T) {
	reg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	for rt, m := range reg {
		if m.IsEdgeMapping() {
			if m.CapabilityLevel != "" {
				t.Errorf("%s: edge mapping must not declare a capability_level, got %q", rt, m.CapabilityLevel)
			}
			continue
		}
		if m.CapabilityLevel == "" {
			t.Errorf("%s: missing capability_level", rt)
			continue
		}
		if !m.CapabilityLevel.Valid() {
			t.Errorf("%s: capability_level %q is not one of core.CapabilityLevel's 9 rungs", rt, m.CapabilityLevel)
			continue
		}
		ceiling := core.MaxImplementedCapabilityLevel(m.NodeType)
		if !ceiling.AtLeast(m.CapabilityLevel) {
			t.Errorf("%s: capability_level %q exceeds the real ceiling %q for node_type %q", rt, m.CapabilityLevel, ceiling, m.NodeType)
		}
	}
}

// TestAWSRegistry_RequestSimulationServicesAreTraceEligible cross-checks PC-107's
// registry against PC-114's own BuildTrace gate: every mapping claiming at least
// REQUEST_SIMULATION must actually be usable as a BuildTrace source/destination once
// ingested — i.e. its node_type's own ceiling really does reach REQUEST_SIMULATION,
// so the claim and the real pipeline gate can never silently diverge.
func TestAWSRegistry_RequestSimulationServicesAreTraceEligible(t *testing.T) {
	reg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	for rt, m := range reg {
		if m.IsEdgeMapping() {
			continue
		}
		if m.CapabilityLevel.AtLeast(core.CapabilityRequestSimulation) {
			if !core.MaxImplementedCapabilityLevel(m.NodeType).AtLeast(core.CapabilityRequestSimulation) {
				t.Errorf("%s: claims REQUEST_SIMULATION but node_type %q's own ceiling doesn't reach it — BuildTrace would never actually accept this service", rt, m.NodeType)
			}
		}
	}
}
