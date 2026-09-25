// This file is PC-125: structural traffic flow — which components carry each
// declared journey, and whether/where it stops under a declared fault. Structural
// only, per the Card's own instruction: paths, not quantities (PC-126 owns load).
//
// Reuses PC-114's BuildTrace for every hop's own SG/NACL/route evaluation — never
// reimplemented here — and PC-14's own containment-closure/reachability functions
// (core.ContainmentBlastRadius, core.SimulateLoss) for the CALLER to compute the
// "killed" set a fault produces; this file only consumes that set, per the Card's own
// "do not duplicate it."
//
// SCOPE LIMITATION, stated rather than silent: the Card asks for "the set of parallel
// paths" a load-balanced/multi-AZ component splits a journey across. This IR's own
// node granularity has no way to express that: a multi-AZ resource (e.g. an EKS
// cluster whose node group spans three subnets) is ONE IR node regardless of how many
// AZs back it, not one node per AZ instance — there is no per-instance node to
// enumerate parallel paths BETWEEN. Building that would mean changing how ingest
// models multi-AZ resources, real, separate, larger work this ticket does not
// attempt. What IS implemented: each hop's own real SG/NACL/route flow (fully
// reusing PC-114, zero duplication) and fault-aware stopping (a hop whose endpoint
// is in the caller-supplied killed set stops the journey there, citing why) — a real,
// complete, working answer to "does this journey flow, and where does it stop,"
// short of enumerating literal parallel AZ paths.
package core

// JourneyInternetSentinel is DeclaredJourney.Path's own reserved first-hop value for
// an internet-originated journey (the Card's own example: "checkout: internet → WAF →
// ALB → ...") — mirrors BuildTrace's own documented convention (empty sourceID, a
// real sourceCIDR) for "internet," which is not a real IR node.
const JourneyInternetSentinel = "internet"

// JourneyHopFlow is one consecutive pair in a journey's declared Path, and whether it
// actually flows.
type JourneyHopFlow struct {
	From    string
	To      string
	Allowed bool
	Reason  string
}

// JourneyFlowResult is one journey's full structural flow outcome.
type JourneyFlowResult struct {
	JourneyID     string
	Flows         bool
	Hops          []JourneyHopFlow
	BlockedAt     string // empty when Flows is true
	BlockedReason string // empty when Flows is true
}

// ComputeJourneyFlow walks j's declared Path hop by hop. killed may be nil (no fault
// currently applied); any node ID present in it is treated as unreachable — the
// journey stops at the first hop touching one, without even consulting BuildTrace
// (an unreachable component has no meaningful SG/NACL/route decision left to make).
// Otherwise each hop's flow is exactly BuildTrace's own Allowed/Concise result,
// reused verbatim, never recomputed independently.
func ComputeJourneyFlow(ir *IR, j DeclaredJourney, killed map[string]bool) JourneyFlowResult {
	result := JourneyFlowResult{JourneyID: j.ID, Flows: true}

	for i := 0; i+1 < len(j.Path); i++ {
		from, to := j.Path[i], j.Path[i+1]

		if from != JourneyInternetSentinel && killed[from] {
			reason := "component " + from + " is unreachable under the declared fault"
			result.Hops = append(result.Hops, JourneyHopFlow{From: from, To: to, Allowed: false, Reason: reason})
			result.Flows, result.BlockedAt, result.BlockedReason = false, from, reason
			return result
		}
		if killed[to] {
			reason := "component " + to + " is unreachable under the declared fault"
			result.Hops = append(result.Hops, JourneyHopFlow{From: from, To: to, Allowed: false, Reason: reason})
			result.Flows, result.BlockedAt, result.BlockedReason = false, to, reason
			return result
		}

		var trace Trace
		if from == JourneyInternetSentinel {
			trace = BuildTrace(ir, "", to, "0.0.0.0/0", j.Protocol, j.Port)
		} else {
			trace = BuildTrace(ir, from, to, "", j.Protocol, j.Port)
		}
		result.Hops = append(result.Hops, JourneyHopFlow{From: from, To: to, Allowed: trace.Allowed, Reason: trace.Concise})
		if !trace.Allowed {
			result.Flows, result.BlockedAt, result.BlockedReason = false, to, trace.Concise
			return result
		}

		// PC-135: the IAM step applies only once every network hop has already
		// passed, and only on the journey's own final hop (see
		// DeclaredJourney.IAMCheck's own doc comment for why) — checked here, inside
		// the loop's own success path, so it naturally only ever runs after the last
		// hop's network trace has already succeeded.
		if j.IAMCheck != nil && i+2 == len(j.Path) {
			iamTrace := BuildTraceWithIAM(ir, from, to, j.IAMCheck.PrincipalID, j.IAMCheck.Action, j.IAMCheck.ResourceARN, "", j.Protocol, j.Port)
			if !iamTrace.Allowed {
				result.Hops[len(result.Hops)-1] = JourneyHopFlow{From: from, To: to, Allowed: false, Reason: iamTrace.Concise}
				result.Flows, result.BlockedAt, result.BlockedReason = false, to, iamTrace.Concise
				return result
			}
		}
	}
	return result
}

// ComputeAllJourneyFlows evaluates every declared journey, in the workload's own
// declared order — deterministic (NFR-1) by construction, since it neither sorts nor
// iterates a map.
func ComputeAllJourneyFlows(ir *IR, workload Workload, killed map[string]bool) []JourneyFlowResult {
	var out []JourneyFlowResult
	for _, j := range workload.Journeys {
		out = append(out, ComputeJourneyFlow(ir, j, killed))
	}
	return out
}
