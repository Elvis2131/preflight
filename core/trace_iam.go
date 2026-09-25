// This file is PC-135's first integration point, verbatim from the Card: "for
// service-to-service calls that require IAM ... add an IAM step after network steps.
// A request that passes every network check but fails IAM is reported failing at the
// IAM step, with the deciding statement."
//
// BuildTraceWithIAM wraps PC-114's own BuildTrace unmodified (every existing caller
// of BuildTrace — journeys, simulate, the MCP trace tool — is untouched) and appends
// exactly one more step, evaluated via PC-134's EvaluateIAMRequest verbatim — this
// file contains zero IAM evaluation logic of its own, per the Card's own boundary
// rule ("consumers call the PC-134 evaluator; none of them reimplement any IAM
// logic").
package core

import "fmt"

// BuildTraceWithIAM runs BuildTrace first; only if every network step passes does it
// go on to evaluate whether principalID may perform action against resourceARN on
// destID (destID doubles as EvaluateIAMRequest's ResourceID, since PC-133 attaches a
// resource-based policy directly to the IR node a request targets). If the network
// portion already fails or is not_assessable, the IAM step is never reached — added
// as an unreached TraceStep so callers see structurally why it was skipped, the same
// "Reached" convention BuildTrace's own steps already use.
func BuildTraceWithIAM(ir *IR, sourceID, destID, principalID, action, resourceARN, sourceCIDR, protocol string, port int) Trace {
	trace := BuildTrace(ir, sourceID, destID, sourceCIDR, protocol, port)
	prov := NewProvenance(KindDerived, fmt.Sprintf("core/trace_iam:%s->%s:%s", principalID, destID, action))

	if !trace.Allowed {
		trace.Steps = append(trace.Steps, TraceStep{
			Step: "iam_check", Component: destID, Operation: fmt.Sprintf("evaluate whether %s may %s on %s", principalID, action, destID),
			Decision: TraceDeny, Reason: "not evaluated — a preceding network step already blocks this request", Reached: false, Provenance: prov,
		})
		return trace
	}

	result := EvaluateIAMRequest(ir, IAMRequest{PrincipalID: principalID, Action: action, ResourceARN: resourceARN, ResourceID: destID}, prov)

	decision := TraceDeny
	reason := fmt.Sprintf("%s may not %s on %s: %s", principalID, action, destID, joinReasoning(result.Reasoning))
	switch result.Decision {
	case IAMDecisionAllow:
		decision = TraceAllow
		reason = fmt.Sprintf("%s is allowed to %s on %s: %s", principalID, action, destID, joinReasoning(result.Reasoning))
	case IAMDecisionNotAssessable:
		decision = TraceNotAssessable
		reason = fmt.Sprintf("whether %s may %s on %s could not be determined: %s", principalID, action, destID, joinReasoning(result.Reasoning))
	}

	rule := ""
	if len(result.DecidingStatements) > 0 {
		rule = fmt.Sprintf("policy %s, statement %s (%s)", result.DecidingStatements[0].PolicyID, statementSidOrNone(result.DecidingStatements[0].Sid), result.DecidingStatements[0].Effect)
	}

	trace.Steps = append(trace.Steps, TraceStep{
		Step: "iam_check", Component: destID, Operation: fmt.Sprintf("evaluate whether %s may %s on %s", principalID, action, destID),
		Decision: decision, Reason: reason, RuleCited: rule, Reached: true, Provenance: prov,
	})
	if decision != TraceAllow {
		trace.Allowed = false
		trace.Concise = fmt.Sprintf("Blocked at %s (iam_check): %s", destID, reason)
	}
	return trace
}

func statementSidOrNone(sid string) string {
	if sid == "" {
		return "<no Sid>"
	}
	return sid
}

func joinReasoning(reasoning []string) string {
	if len(reasoning) == 0 {
		return "no reason recorded"
	}
	out := reasoning[0]
	for _, r := range reasoning[1:] {
		out += "; " + r
	}
	return out
}
