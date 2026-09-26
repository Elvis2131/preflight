// This file is PC-126: per-component offered load, utilisation, and a ranked
// bottleneck list, computed from PC-124's declared journeys/capacity and PC-125's
// structural flow — never a second load-computation path, and never a composite
// score (only utilisation per component, ranked, same "no single number" discipline
// PC-19 already established for the scorecard).
//
// CAPACITY DISCIPLINE, carried over unchanged from core/internal/analyse/capacity.go
// (CLAUDE.md §9): capacity is declared-only. Sizing.Count (PC-115) or any other
// instance-count fact is NEVER multiplied into a capacity figure by this file —
// proven structurally, core/load_test.go's own grep-style check.
//
// LOAD-DIVISION ASSUMPTION, recorded once here rather than left implicit (the Card's
// own instruction): load across a journey's own parallel path members (PC-125's own
// "|"-separated Path groups) divides evenly among however many of that position's
// members are actually reached right now — under a fault that kills some but not all
// of a group, the SAME total flow continues through fewer members, each now carrying
// more (the Card's own named example: "2 of 3 app nodes lost, survivors at 150% of
// declared capacity"). Never assumed to be AWS's own real ALB/routing algorithm;
// stated as a modelling simplification, applied identically regardless of which
// specific members survive.
package core

import (
	"fmt"
	"sort"
)

// LoadDivisionAssumption is PC-126's own recorded rule for dividing load across a
// journey's own parallel path members — see this file's own package doc comment. Now
// genuinely exercised (core/load_parallel_test.go) since core/journey_flow.go gained
// real "|"-separated parallel Path groups.
const LoadDivisionAssumption = "even split across parallel paths (not AWS's own real load-balancing algorithm — a stated modelling simplification)"

// ComponentLoad is one component's offered load and (if capacity is declared)
// utilisation, for one evaluated scenario (no fault, or a specific declared one).
type ComponentLoad struct {
	NodeID              string
	NodeType            NodeType
	OfferedRPS          float64
	CapacityKey         string
	Capacity            *float64 // nil when undeclared — never a guessed default
	Utilization         *float64 // nil when Capacity is nil
	NotAssessableReason string   // populated only when Capacity is nil

	// LoadDivisionNote is PC-126's own explicit "visible in output" acceptance
	// criterion: populated with LoadDivisionAssumption's own text, but ONLY for a
	// component that is actually a member of a currently-reached parallel group of
	// more than one (i.e. OfferedRPS above genuinely reflects an even-split
	// assumption, not just a linear pass-through) — empty for every other component,
	// so this never implies an assumption was applied where it wasn't.
	LoadDivisionNote string
}

// ComputeComponentLoad sums every fully-specified journey's (declared PeakRPS, PC-124)
// load onto every node it actually reaches, under the given fault (killed may be nil
// for no fault — same convention as core.ComputeJourneyFlow). A node is credited with
// a journey's load only for hops that actually flow (JourneyHopFlow.Allowed) —
// traffic that never arrives at a node contributes no load to it, and a killed node
// is reported unreachable by ComputeJourneyFlow itself, never assigned a load figure
// implying it is still handling requests.
func ComputeComponentLoad(ir *IR, workload Workload, killed map[string]bool) []ComponentLoad {
	byID := make(map[string]Node, len(ir.Nodes))
	for _, n := range ir.Nodes {
		byID[n.ID] = n
	}

	offered := map[string]float64{}
	touched := map[string]bool{}
	divided := map[string]bool{}
	var order []string

	for _, j := range workload.Journeys {
		if j.PeakRPS == nil {
			continue // no declared peak load — this journey contributes nothing countable, never guessed
		}
		flow := ComputeJourneyFlow(ir, j, killed)
		// Each reached member of each Path position is credited EXACTLY ONCE, with
		// *j.PeakRPS divided by however many members of that same position were
		// actually reached (LoadDivisionAssumption above) — crediting per Hops entry
		// instead would double- (or N-) count a node that is the source or
		// destination of more than one evaluated pair in a parallel group (e.g. three
		// sources fanning into one shared destination is 3 Hops entries but ONE real
		// arrival of the journey's own total rate at that destination). A singleton
		// position (len 1, every pre-parallel-paths journey) divides by 1, crediting
		// the full declared rate — this function's own original behaviour, unchanged.
		for _, members := range flow.ReachedByGroup {
			if len(members) == 0 {
				continue
			}
			share := *j.PeakRPS / float64(len(members))
			for _, nodeID := range members {
				if nodeID == JourneyInternetSentinel {
					continue // not a real component
				}
				if !touched[nodeID] {
					touched[nodeID] = true
					order = append(order, nodeID)
				}
				offered[nodeID] += share
				if len(members) > 1 {
					divided[nodeID] = true
				}
			}
		}
	}

	sort.Strings(order) // deterministic (NFR-1): output order never depends on journey/hop iteration order

	out := make([]ComponentLoad, 0, len(order))
	for _, nodeID := range order {
		node := byID[nodeID]
		key := JourneyCapacityKey(node.Type)
		cl := ComponentLoad{NodeID: nodeID, NodeType: node.Type, OfferedRPS: offered[nodeID], CapacityKey: key}
		if divided[nodeID] {
			cl.LoadDivisionNote = LoadDivisionAssumption
		}
		if capValue, ok := workload.Capacity[key]; ok {
			util := cl.OfferedRPS / capValue
			cl.Capacity = &capValue
			cl.Utilization = &util
		} else {
			cl.NotAssessableReason = fmt.Sprintf("no declared capacity for node type %q (expected Workload.Capacity[%q])", node.Type, key)
		}
		out = append(out, cl)
	}
	return out
}

// RankBottlenecks returns only components with a real, assessed utilisation, sorted
// descending by utilisation (the highest-utilisation component first) — a ranked
// LIST, never collapsed into one composite score.
func RankBottlenecks(loads []ComponentLoad) []ComponentLoad {
	var assessed []ComponentLoad
	for _, l := range loads {
		if l.Utilization != nil {
			assessed = append(assessed, l)
		}
	}
	sort.SliceStable(assessed, func(i, j int) bool {
		return *assessed[i].Utilization > *assessed[j].Utilization
	})
	return assessed
}
