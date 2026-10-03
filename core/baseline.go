package core

import "fmt"

// PC-161: baseline validity. A fault simulation is only meaningful for traffic that flowed BEFORE the
// fault. Without this, a journey that never worked (a wrong port, a missing rule) came back from
// /simulate as part of a design that was "unaffected", because the headline verdict is graph
// reachability that ignores ports, security groups, network ACLs and routes. The baseline is the same
// per-journey flow engine run on the design as drawn, with no fault, kept separate from the result
// after the fault so "was already broken" can never be mistaken for "survived" or "broken by the fault".

// JourneyBaselineStatus is exactly one of these per declared journey.
type JourneyBaselineStatus string

const (
	// BaselineFlowsBeforeAndAfter: the journey flowed before the fault and still does.
	BaselineFlowsBeforeAndAfter JourneyBaselineStatus = "flows_before_and_after"
	// BaselineAlreadyBlocked: the journey did not flow before any fault. It is never counted as having
	// survived; the hop and reason are those of the design as drawn.
	BaselineAlreadyBlocked JourneyBaselineStatus = "already_blocked"
	// BaselineBrokenByFault: it flowed before and does not now.
	BaselineBrokenByFault JourneyBaselineStatus = "broken_by_fault"
	// BaselineDegradedByFault: it flowed before; now only its declared fallback path flows.
	BaselineDegradedByFault JourneyBaselineStatus = "degraded_by_fault"
	// BaselineNotAssessable: the engine could not decide whether it flowed before the fault (an
	// unreadable or unmodelled input). Never reported as blocked and never as flowing (I4).
	BaselineNotAssessable JourneyBaselineStatus = "not_assessable"
)

// Summary values when the baseline summary is assessed.
const (
	BaselineAllDeclaredJourneysFlow     = "all_declared_journeys_flow"
	BaselineSomeDeclaredJourneysBlocked = "some_declared_journeys_blocked"
)

// JourneyBaseline is one journey's before/after, side by side.
type JourneyBaseline struct {
	JourneyID string                `json:"journey_id"`
	Status    JourneyBaselineStatus `json:"status"`

	// BaselineBlockedAt/BaselineReason: where and why the journey stopped in the design as drawn.
	// Set for already_blocked and for not_assessable.
	BaselineBlockedAt string `json:"baseline_blocked_at,omitempty"`
	BaselineReason    string `json:"baseline_reason,omitempty"`

	// AfterFaultBlockedAt/AfterFaultReason: where and why it stopped under the fault. Set for
	// broken_by_fault and degraded_by_fault.
	AfterFaultBlockedAt string `json:"after_fault_blocked_at,omitempty"`
	AfterFaultReason    string `json:"after_fault_reason,omitempty"`

	DegradedVia string `json:"degraded_via,omitempty"`
}

// BaselineValidity answers "does this design carry the declared traffic at all?" before any fault is
// considered.
type BaselineValidity struct {
	// Summary is assessed (all_declared_journeys_flow | some_declared_journeys_blocked) or
	// not_assessable with the reason: no journey declared, or a baseline that could not be decided.
	Summary AssessmentEnvelope `json:"summary"`
	// Journeys is one entry per declared journey, in declared order; never nil.
	Journeys []JourneyBaseline `json:"journeys"`
}

// ComputeBaselineValidity compares each declared journey's flow in the unfaulted design (before)
// with its flow under the fault (after). before and after must be in the workload's declared order.
func ComputeBaselineValidity(workload Workload, before, after []JourneyFlowResult, prov Provenance) BaselineValidity {
	out := BaselineValidity{Journeys: make([]JourneyBaseline, 0, len(before))}
	if len(workload.Journeys) == 0 {
		out.Summary = NotAssessable[any]("no journey is declared, so whether traffic flows through this design was not checked; the verdict reports graph reachability only and says nothing about ports, security groups, network ACLs or routes", prov).ToEnvelope()
		return out
	}
	var blocked, undecided []string
	for i, b := range before {
		jb := JourneyBaseline{JourneyID: b.JourneyID}
		var a JourneyFlowResult
		if i < len(after) {
			a = after[i]
		}
		switch {
		case !b.Flows && b.NotAssessable:
			jb.Status = BaselineNotAssessable
			jb.BaselineBlockedAt, jb.BaselineReason = b.BlockedAt, b.BlockedReason
			undecided = append(undecided, fmt.Sprintf("%s (%s)", b.JourneyID, b.BlockedReason))
		case !b.Flows:
			jb.Status = BaselineAlreadyBlocked
			jb.BaselineBlockedAt, jb.BaselineReason = b.BlockedAt, b.BlockedReason
			blocked = append(blocked, fmt.Sprintf("%s (%s)", b.JourneyID, b.BlockedReason))
		case a.Flows:
			jb.Status = BaselineFlowsBeforeAndAfter
		case a.Degraded:
			jb.Status = BaselineDegradedByFault
			jb.AfterFaultBlockedAt, jb.AfterFaultReason, jb.DegradedVia = a.BlockedAt, a.BlockedReason, a.DegradedVia
		default:
			jb.Status = BaselineBrokenByFault
			jb.AfterFaultBlockedAt, jb.AfterFaultReason = a.BlockedAt, a.BlockedReason
		}
		out.Journeys = append(out.Journeys, jb)
	}
	switch {
	case len(blocked) > 0:
		out.Summary = Assessed[any](BaselineSomeDeclaredJourneysBlocked, prov).ToEnvelope()
	case len(undecided) > 0:
		out.Summary = NotAssessable[any](fmt.Sprintf("the baseline of %d declared journey(s) could not be decided: %v", len(undecided), undecided), prov).ToEnvelope()
	default:
		out.Summary = Assessed[any](BaselineAllDeclaredJourneysFlow, prov).ToEnvelope()
	}
	return out
}

// baselineBlockedReason is the text Simulate puts on a verdict it refuses to give: a design known not
// to carry a declared journey cannot be said to have "survived" a fault.
func baselineBlockedReason(b BaselineValidity) string {
	var parts []string
	for _, j := range b.Journeys {
		if j.Status == BaselineAlreadyBlocked {
			parts = append(parts, fmt.Sprintf("journey %q was already blocked before the fault at %s (%s)", j.JourneyID, j.BaselineBlockedAt, j.BaselineReason))
		}
	}
	return fmt.Sprintf("%v; a survival verdict for a design that never carried this traffic would be meaningless. See baseline for each journey; severed_paths, cascade and journeys are structural facts and are still reported", parts)
}
