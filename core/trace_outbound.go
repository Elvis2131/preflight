// This file is PC-155: an outbound hop. A declared journey may end at the internet
// sentinel (the workload calling an external payment rail through a NAT gateway).
//
// "internet" is not an IR node, so BuildTrace's destination lookup cannot represent it.
// BuildOutboundTrace evaluates only what the design itself decides, in the order AWS
// applies it on the way out: the source subnet's default route (EvaluateEgressRoute, with
// the route's target still required to exist, which is how NAT loss breaks the journey),
// the source subnet's NACL (outbound leg plus the stateless return leg), and the source's
// security groups (outbound rule; the response is allowed because SGs are stateful).
//
// The far side is outside the design. Its own network behaviour is never modelled; the
// final step says so and does not gate the verdict (I4: it is stated as out of scope, not
// guessed allow or deny).
package core

import "fmt"

// BuildOutboundTrace traces sourceID -> the internet on protocol/port.
func BuildOutboundTrace(ir *IR, sourceID, protocol string, port int) Trace {
	prov := NewProvenance(KindDerived, fmt.Sprintf("core/trace_outbound:%s->internet:%s/%d", sourceID, protocol, port))
	trace := Trace{Source: sourceID, Destination: JourneyInternetSentinel, Protocol: protocol, Port: port}
	reached := true
	step := func(name, component, operation string, decision TraceDecision, reason, rule string) {
		trace.Steps = append(trace.Steps, TraceStep{Step: name, Component: component, Operation: operation, Decision: decision, Reason: reason, RuleCited: rule, Reached: reached, Provenance: prov})
		if reached && decision != TraceAllow {
			reached = false
		}
	}

	src, ok := findNode(ir, sourceID)
	if !ok {
		step("resolve_source", sourceID, "look up the source node in the IR", TraceDeny, "source node not found in the IR", "")
		return finalizeOutbound(trace)
	}
	step("resolve_source", sourceID, "look up the source node in the IR", TraceAllow, "source node exists", "")

	if !requestSimulationCapable(src) {
		step("capability_check", sourceID, "check whether this service supports request simulation", TraceNotAssessable,
			fmt.Sprintf("service %s (capability_level %v) does not reach REQUEST_SIMULATION", sourceID, src.RawAttributes["capability_level"]), "")
		return finalizeOutbound(trace)
	}
	step("capability_check", sourceID, "check whether this service supports request simulation", TraceAllow, "service type is modelled for request simulation", "")

	subnetID, hasSubnet := resolveSubnetID(ir, sourceID)
	if !hasSubnet {
		step("route_selection", sourceID, "resolve the source's effective route table", TraceNotAssessable, "source has no resolvable effective route table", "")
		return finalizeOutbound(trace)
	}

	// Route: the default route must exist and its target must still exist.
	allowed, targetKind, reason := EvaluateEgressRoute(ir, subnetID)
	switch {
	case !allowed:
		step("route_selection", subnetID, "select the subnet's default route (0.0.0.0/0) toward the internet", TraceDeny, reason, "")
		return finalizeOutbound(trace)
	case targetKind != "nat_gateway":
		// An internet gateway route only carries traffic for a resource with a public IPv4
		// address or Elastic IP, which the IR does not model; never guessed.
		step("route_selection", subnetID, "select the subnet's default route (0.0.0.0/0) toward the internet", TraceNotAssessable,
			"the default route targets a "+orUnknown(targetKind)+", not a NAT gateway; whether the source has a public address to use it is not modelled", "")
		return finalizeOutbound(trace)
	}
	step("route_selection", subnetID, "select the subnet's default route (0.0.0.0/0) toward the internet", TraceAllow, reason, "")

	// NACL: the outbound rule on the source subnet, and the stateless return leg. The
	// internet side has no NACL of its own, so a permissive placeholder stands in for it.
	srcRes, hasNACL := ResolveSubnetNACL(ir.Nodes, ir.Edges, subnetID)
	if !hasNACL {
		step("nacl_check", subnetID, "resolve the associated NACL for the source subnet", TraceNotAssessable, "source subnet has no resolvable associated NACL", "")
		return finalizeOutbound(trace)
	}
	open := NACLProfile{NACLID: "internet", Rules: []NACLRule{
		{Number: NACLCatchAll, Direction: "ingress", Protocol: "-1", CIDR: "0.0.0.0/0", Allow: true},
		{Number: NACLCatchAll, Direction: "egress", Protocol: "-1", CIDR: "0.0.0.0/0", Allow: true},
	}}
	srcCIDR := "0.0.0.0/0"
	if c, ok := subnetCIDR(ir, subnetID); ok {
		srcCIDR = c
	}
	_, naclOK, fwd, ret := EvaluateNACLPath(subnetID, "internet", srcRes.Profile, open, srcCIDR, "0.0.0.0/0", protocol, port, 0, 0)
	assumedNote := ""
	if srcRes.Source == NACLAssumedDefault {
		assumedNote = "; " + NACLAssumedDefaultReason
	}
	step("nacl_source_egress", srcRes.Profile.NACLID, "evaluate source subnet's NACL, outbound", boolDecision(fwd[0].Allowed), fwd[0].Reason+assumedNote, "NACL "+fwd[0].NACLID+" rule "+fwd[0].MatchedRuleNumber)
	if !fwd[0].Allowed {
		return finalizeOutbound(trace)
	}
	if !naclOK {
		step("nacl_response_path", srcRes.Profile.NACLID, "evaluate the stateless return path (source subnet inbound, ephemeral ports)", TraceDeny, "return traffic denied: "+ret[1].Reason+assumedNote, "")
		return finalizeOutbound(trace)
	}

	// Security group: outbound rule on the source; a response is allowed back (stateful).
	srcSG := SecurityGroupProfile(ir.Nodes, ir.Edges, sourceID)
	if len(srcSG.SGIDs) == 0 {
		step("sg_source_egress", sourceID, "evaluate source's security groups, outbound (union of all attached SGs)", TraceNotAssessable,
			"no security group is attached to "+sourceID+" — its own outbound posture is unknown, never assumed", "")
		return finalizeOutbound(trace)
	}
	openSG := SGProfile{SGIDs: []string{"internet"}, Rules: []SGRule{{Direction: "ingress", Protocol: "-1", CIDRs: []string{"0.0.0.0/0"}}}}
	okConn, initD, _ := EvaluateConnection(srcSG, openSG, srcCIDR, "0.0.0.0/0", protocol, port)
	step("sg_source_egress", sourceID, "evaluate source's security groups, outbound (union of all attached SGs)", boolDecision(okConn), sgReasonOrDefault(initD), "SG "+initD.MatchedSG)
	if !okConn {
		return finalizeOutbound(trace)
	}

	step("internet_boundary", JourneyInternetSentinel, "the request leaves the design at the internet boundary", TraceAllow,
		"the far side is outside the design; its own network behaviour is not modelled and nothing is claimed about it", "")
	return finalizeOutbound(trace)
}

func orUnknown(s string) string {
	if s == "" {
		return "target of unknown kind"
	}
	return s
}

func finalizeOutbound(t Trace) Trace {
	t = finalize(t)
	if t.Allowed {
		t.Concise = fmt.Sprintf("Allowed: %s -> internet:%d/%s leaves the design.", t.Source, t.Port, t.Protocol)
	}
	return t
}
