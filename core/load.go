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
// own instruction): if/when PC-125 grows real parallel-path enumeration (its own
// stated scope gap — this IR's node granularity has no per-AZ-instance node today),
// load across those parallel paths would divide evenly among them. That rule is not
// yet exercised — PC-125 does not produce parallel paths to divide across — but is
// recorded now so it has exactly one place to live when it becomes real, rather than
// being invented ad hoc later. Never assumed to be AWS's own real ALB/routing
// algorithm; stated as a modelling simplification.
package core

import (
	"fmt"
	"sort"
)

// LoadDivisionAssumption is PC-126's own recorded (currently unexercised — see this
// file's own package doc comment) rule for dividing load across parallel paths, once
// PC-125 can produce more than one per journey.
const LoadDivisionAssumption = "even split across parallel paths (not AWS's own real load-balancing algorithm — a stated modelling simplification, unexercised until PC-125 models parallel paths)"

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
	var order []string

	for _, j := range workload.Journeys {
		if j.PeakRPS == nil {
			continue // no declared peak load — this journey contributes nothing countable, never guessed
		}
		flow := ComputeJourneyFlow(ir, j, killed)
		for _, hop := range flow.Hops {
			if !hop.Allowed {
				continue
			}
			for _, nodeID := range []string{hop.From, hop.To} {
				if nodeID == JourneyInternetSentinel {
					continue // not a real component
				}
				if !touched[nodeID] {
					touched[nodeID] = true
					order = append(order, nodeID)
				}
				offered[nodeID] += *j.PeakRPS
			}
		}
	}

	sort.Strings(order) // deterministic (NFR-1): output order never depends on journey/hop iteration order

	out := make([]ComponentLoad, 0, len(order))
	for _, nodeID := range order {
		node := byID[nodeID]
		key := JourneyCapacityKey(node.Type)
		cl := ComponentLoad{NodeID: nodeID, NodeType: node.Type, OfferedRPS: offered[nodeID], CapacityKey: key}
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
