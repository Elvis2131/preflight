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
// PARALLEL PATHS (added under PC-125/126's own follow-up, after the ticket's initial
// pass shipped without them): a single Path element may name more than one node ID,
// joined by "|" (JourneyParallelSeparator) — e.g. Path: {"internet", "alb",
// "app1|app2|app3", "db"} declares that the app tier's three instances are parallel
// alternatives at that position, not three separate journeys. This is deliberately
// additive to the wire shape (Path stays []string, no contracts/workload.schema.json
// version bump needed) — a Path with no "|" anywhere behaves identically to before
// this change, verified by every pre-existing test in this package still passing
// unchanged. This does NOT lift the underlying IR limitation this file's own earlier
// version documented (a real multi-AZ Terraform resource still ingests as one IR
// node) — it gives an architect a way to EXPLICITLY declare known-redundant
// components (e.g. three separately-modelled replica nodes) as parallel members of
// one journey position, which the engine then evaluates and load-divides across for
// real, rather than requiring a real per-AZ-instance IR redesign to do it.
package core

import "sort"

// JourneyInternetSentinel is DeclaredJourney.Path's own reserved first-hop value for
// an internet-originated journey (the Card's own example: "checkout: internet → WAF →
// ALB → ...") — mirrors BuildTrace's own documented convention (empty sourceID, a
// real sourceCIDR) for "internet," which is not a real IR node.
const JourneyInternetSentinel = "internet"

// JourneyParallelSeparator splits one Path element into its parallel member node
// IDs — see this file's own package doc comment.
const JourneyParallelSeparator = "|"

// splitJourneyHopGroup parses one Path element into its member node IDs, trimmed and
// deduplicated. A hop with no separator returns a single-element slice — the
// singleton case every pre-existing caller and test already exercises.
func splitJourneyHopGroup(hop string) []string {
	var out []string
	seen := map[string]bool{}
	start := 0
	for i := 0; i <= len(hop); i++ {
		if i == len(hop) || hop[i:i+1] == JourneyParallelSeparator {
			member := trimSpace(hop[start:i])
			if member != "" && !seen[member] {
				seen[member] = true
				out = append(out, member)
			}
			start = i + 1
		}
	}
	return out
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

func sortedStrings(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	sort.Strings(out)
	return out
}

// JourneyHopFlow is one evaluated (source, destination) pair belonging to one
// transition in a journey's declared Path, and whether it actually flows. GroupIndex
// is the destination's own position in Path (1-indexed from the journey's start,
// e.g. 1 for the first transition's destination) — core.ComputeComponentLoad uses it
// to find how many parallel members of that same position were actually reached, to
// divide load across them (PC-126's own LoadDivisionAssumption, now exercised for
// real). For a singleton (non-"|") Path, exactly one JourneyHopFlow is recorded per
// transition, identical to this file's pre-parallel-paths behaviour.
type JourneyHopFlow struct {
	From       string
	To         string
	Allowed    bool
	Reason     string
	GroupIndex int
}

// JourneyFlowResult is one journey's full structural flow outcome. ReachedByGroup
// mirrors Path position-for-position (len(ReachedByGroup) == len(Path) whenever the
// journey's own start is reachable at all): ReachedByGroup[i] is the sorted list of
// Path[i]'s own declared members that were actually reached, given SG/NACL/route
// evaluation and the caller-supplied killed set. A singleton Path element that is
// reached always appears as a single-element slice here.
type JourneyFlowResult struct {
	JourneyID      string
	Flows          bool
	Hops           []JourneyHopFlow
	ReachedByGroup [][]string
	BlockedAt      string // empty when Flows is true
	BlockedReason  string // empty when Flows is true

	// Degraded is PC-131's own addition: the primary path does not flow (Flows is
	// false, BlockedAt/BlockedReason still describe why) BUT the journey declared a
	// Fallback and that fallback path really does flow under the same fault.
	// DegradedVia is that fallback's own declared description. Never set for a
	// journey with no declared fallback — graceful degradation is never assumed.
	// Load (ComputeComponentLoad) credits only primary-path flow, so a degraded
	// journey's fallback path carries no modelled load — a stated limit, not a claim.
	Degraded    bool   `json:",omitempty"`
	DegradedVia string `json:",omitempty"`
}

// ComputeJourneyFlow walks j's declared Path hop group by hop group. killed may be
// nil (no fault currently applied); any node ID present in it is treated as
// unreachable — a hop candidate touching one stops there without even consulting
// BuildTrace (an unreachable component has no meaningful SG/NACL/route decision left
// to make). Otherwise each hop's flow is exactly BuildTrace's own Allowed/Concise
// result, reused verbatim, never recomputed independently. A parallel group (see
// this file's own package doc comment) is fully evaluated — every reached member of
// the previous position against every declared member of the current one — and Flows
// as a whole only stops once an ENTIRE position's group has zero reached members.
func ComputeJourneyFlow(ir *IR, j DeclaredJourney, killed map[string]bool) JourneyFlowResult {
	result := computeJourneyPathFlow(ir, j, killed)
	if result.Flows || j.Fallback == nil {
		return result
	}
	// The primary path does not flow. A declared fallback is evaluated by this very
	// same engine under this very same fault; the journey's own IAMCheck is kept on
	// it, which can only make "degraded" harder to claim, never easier (I4: when in
	// doubt, fail rather than overclaim).
	fb := j
	fb.Path = j.Fallback.Path
	fb.Fallback = nil
	if computeJourneyPathFlow(ir, fb, killed).Flows {
		result.Degraded = true
		result.DegradedVia = j.Fallback.Description
	}
	return result
}

// computeJourneyPathFlow is ComputeJourneyFlow's own original body: one declared Path,
// no fallback logic.
func computeJourneyPathFlow(ir *IR, j DeclaredJourney, killed map[string]bool) JourneyFlowResult {
	result := JourneyFlowResult{JourneyID: j.ID, Flows: true}

	groups := make([][]string, len(j.Path))
	for i, hop := range j.Path {
		groups[i] = splitJourneyHopGroup(hop)
	}

	// Position 0 is the journey's own declared start — trivially "reached" except for
	// a member the caller-supplied fault has already killed (mirrors this file's own
	// pre-parallel-paths behaviour: killed[Path[0]] was already checked on the very
	// first loop iteration below, before this rewrite existed).
	var reached []string
	for _, m := range groups[0] {
		if m == JourneyInternetSentinel || !killed[m] {
			reached = append(reached, m)
		}
	}
	result.ReachedByGroup = append(result.ReachedByGroup, sortedStrings(reached))
	if len(reached) == 0 {
		rep := sortedStrings(groups[0])[0]
		reason := "component " + rep + " is unreachable under the declared fault"
		result.Flows, result.BlockedAt, result.BlockedReason = false, rep, reason
		return result
	}

	for i := 0; i+1 < len(groups); i++ {
		sources := sortedStrings(reached)
		isFinalTransition := i+2 == len(groups)

		var nextReached []string
		reasonFor := map[string]string{}

		hopPort := j.PortForHop(j.Path[i+1])
		for _, d := range sortedStrings(groups[i+1]) {
			if killed[d] {
				reason := "component " + d + " is unreachable under the declared fault"
				from := ""
				if len(sources) > 0 {
					from = sources[0]
				}
				result.Hops = append(result.Hops, JourneyHopFlow{From: from, To: d, Allowed: false, Reason: reason, GroupIndex: i + 1})
				reasonFor[d] = reason
				continue
			}
			if len(sources) == 0 {
				reason := "no member of the previous hop group survived to reach this component"
				result.Hops = append(result.Hops, JourneyHopFlow{From: "", To: d, Allowed: false, Reason: reason, GroupIndex: i + 1})
				reasonFor[d] = reason
				continue
			}

			networkAllowed := false
			var lastSource, lastReason string
			for _, s := range sources {
				var trace Trace
				if d == JourneyInternetSentinel {
					// PC-155: an outbound hop leaves the design; there is no destination node.
					trace = BuildOutboundTrace(ir, s, j.Protocol, hopPort)
				} else if s == JourneyInternetSentinel {
					trace = BuildTrace(ir, "", d, "0.0.0.0/0", j.Protocol, hopPort)
				} else {
					trace = BuildTrace(ir, s, d, "", j.Protocol, hopPort)
				}
				result.Hops = append(result.Hops, JourneyHopFlow{From: s, To: d, Allowed: trace.Allowed, Reason: trace.Concise, GroupIndex: i + 1})
				lastSource, lastReason = s, trace.Concise
				if trace.Allowed {
					networkAllowed = true
				}
			}
			if !networkAllowed {
				reasonFor[d] = lastReason
				continue
			}

			// PC-135: the IAM step applies only once every network hop has already
			// passed, and only on the journey's own final hop group (see
			// DeclaredJourney.IAMCheck's own doc comment for why). IAM's own outcome
			// does not depend on which specific source reached d (EvaluateIAMRequest
			// takes no source parameter), so evaluating it once per destination —
			// rather than once per (source, destination) pair — is not a shortcut,
			// it is the actual real answer.
			if isFinalTransition && j.IAMCheck != nil {
				iamTrace := BuildTraceWithIAM(ir, lastSource, d, j.IAMCheck.PrincipalID, j.IAMCheck.Action, j.IAMCheck.ResourceARN, "", j.Protocol, hopPort)
				if !iamTrace.Allowed {
					result.Hops = append(result.Hops, JourneyHopFlow{From: lastSource, To: d, Allowed: false, Reason: iamTrace.Concise, GroupIndex: i + 1})
					reasonFor[d] = iamTrace.Concise
					continue
				}
			}

			nextReached = append(nextReached, d)
		}

		result.ReachedByGroup = append(result.ReachedByGroup, sortedStrings(nextReached))
		if len(nextReached) == 0 {
			rep := sortedStrings(groups[i+1])[0]
			result.Flows, result.BlockedAt, result.BlockedReason = false, rep, reasonFor[rep]
			return result
		}
		reached = nextReached
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
