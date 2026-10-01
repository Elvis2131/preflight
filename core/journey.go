// This file is PC-124's own acceptance criterion, made real: "Missing rps or capacity
// produce not_assessable load results naming the missing field." Full load
// distribution and bottleneck detection is PC-125/126's own job; this is the
// structural readiness gate those tickets build on — a journey either has everything
// declared that a real load computation needs, or it is honestly not_assessable,
// naming exactly what is missing, never a guessed default.
package core

import "fmt"

// JourneyCapacityKey is PC-124's own recorded convention for Workload.Capacity's
// keys when resolving a journey's per-component capacity: the component's own
// canonical NodeType string, suffixed "_rps". This is a NEW, GENERAL convention, not
// a pre-existing general one — core/simulate.go's own SurvivingCapacity call already
// reads this same map, but via one single hardcoded key ("app_node_rps", scoped only
// to container_workload's own PC-82 capacity check), not a general per-NodeType
// scheme any caller can resolve for any component. That existing key is left exactly
// as-is (renaming it would break core/simulate_golden_test.go); this convention is
// additive, for journeys' own (and PC-125/126/128's future) per-component lookups.
// Recorded once here rather than reinvented ad hoc by each future caller.
func JourneyCapacityKey(t NodeType) string {
	return string(t) + "_rps"
}

// EvaluateJourneyLoadReadiness reports whether j has every declared input a real load
// computation needs: its own peak_rps/steady_rps, every path component resolving to
// a real IR node, and a declared Workload.Capacity entry (JourneyCapacityKey) for
// each such component's node type. The first missing input found is reported by
// name — not_assessable, never a guess standing in for the gap.
func EvaluateJourneyLoadReadiness(ir *IR, workload Workload, j DeclaredJourney, prov Provenance) AssessmentEnvelope {
	if j.PeakRPS == nil {
		return NotAssessable[any](fmt.Sprintf("journey %q has no declared peak_rps", j.ID), prov).ToEnvelope()
	}
	if j.SteadyRPS == nil {
		return NotAssessable[any](fmt.Sprintf("journey %q has no declared steady_rps", j.ID), prov).ToEnvelope()
	}

	byID := make(map[string]Node, len(ir.Nodes))
	for _, n := range ir.Nodes {
		byID[n.ID] = n
	}

	// A Path element may be a parallel group ("db|db2", JourneyParallelSeparator —
	// PC-125/126); every member is a real node needing its own declared capacity. The
	// readiness gate originally looked the whole "db|db2" string up as one node ID and
	// so could never pass for a parallel-path journey (found by PC-128's latency tests).
	for _, hop := range j.Path {
		for _, nodeID := range splitJourneyHopGroup(hop) {
			if nodeID == JourneyInternetSentinel {
				// Not a real IR node by design (core/journey_flow.go's own convention,
				// mirroring BuildTrace's) — has no node type and needs no declared
				// capacity of its own.
				continue
			}
			node, ok := byID[nodeID]
			if !ok {
				return NotAssessable[any](fmt.Sprintf("journey %q references component %q, which does not exist in this IR", j.ID, nodeID), prov).ToEnvelope()
			}
			key := JourneyCapacityKey(node.Type)
			if _, hasCapacity := workload.Capacity[key]; !hasCapacity {
				return NotAssessable[any](fmt.Sprintf("journey %q has no declared capacity for component %q (expected Workload.Capacity[%q])", j.ID, nodeID, key), prov).ToEnvelope()
			}
		}
	}

	return Assessed[any](fmt.Sprintf("journey %q has every input declared for load computation", j.ID), prov).ToEnvelope()
}
