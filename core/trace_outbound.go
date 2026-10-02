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

import (
	"fmt"
	"strings"
)

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

	// An undeterminable route table (an association or route ingest could not read, or no declared main
	// table) is not a denial: the engine cannot tell, so it says so (PC-158).
	if _, _, resolvable := ResolveSubnetRouteTable(ir, subnetID); !resolvable {
		step("route_selection", subnetID, "select the subnet's default route (0.0.0.0/0) toward the internet", TraceNotAssessable,
			"subnet has no resolvable effective route table (an association or route could not be read, or the design declares no main route table)", "")
		return finalizeOutbound(trace)
	}
	// Route: the default route must exist and its target must still exist.
	allowed, targetKind, reason := EvaluateEgressRoute(ir, subnetID)
	switch {
	case !allowed:
		step("route_selection", subnetID, "select the subnet's default route (0.0.0.0/0) toward the internet", TraceDeny, reason, "")
		return finalizeOutbound(trace)
	case targetKind != "nat_gateway" && targetKind != "internet_gateway":
		step("route_selection", subnetID, "select the subnet's default route (0.0.0.0/0) toward the internet", TraceNotAssessable,
			"the default route targets a "+orUnknown(targetKind)+", which is not modelled as an outbound path", "")
		return finalizeOutbound(trace)
	}
	step("route_selection", subnetID, "select the subnet's default route (0.0.0.0/0) toward the internet", TraceAllow, reason, "")

	// An internet gateway only carries IPv4 traffic for a resource that holds a public IPv4
	// address (a NAT gateway supplies its own, so that route needs no such check).
	if targetKind == "internet_gateway" {
		pa := ResolvePublicIPv4(ir, sourceID, subnetID)
		pubProv := prov
		if pa.Assumed {
			pubProv = NewProvenance(KindAssumed, "core/trace_outbound:"+subnetID+":map_public_ip_on_launch-default").WithReason(pa.Reason)
		}
		trace.Steps = append(trace.Steps, TraceStep{Step: "public_address", Component: sourceID,
			Operation: "check the source holds a public IPv4 address (an internet gateway only carries IPv4 traffic for one)",
			Decision:  pa.Decision, Reason: pa.Reason, RuleCited: pa.Rule, Reached: reached, Provenance: pubProv})
		if pa.Decision != TraceAllow {
			return finalizeOutbound(trace)
		}
	}

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
	step("nacl_source_egress", srcRes.Profile.NACLID, "evaluate source subnet's NACL, outbound", naclStepDecision(fwd[0]), fwd[0].Reason+assumedNote, "NACL "+fwd[0].NACLID+" rule "+fwd[0].MatchedRuleNumber)
	if !fwd[0].Allowed {
		return finalizeOutbound(trace)
	}
	if !naclOK {
		step("nacl_response_path", srcRes.Profile.NACLID, "evaluate the stateless return path (source subnet inbound, ephemeral ports)", naclStepDecision(ret[1]), "return traffic not permitted: "+ret[1].Reason+assumedNote, "")
		return finalizeOutbound(trace)
	}

	// Security group: outbound rule on the source; a response is allowed back (stateful).
	srcSG := SecurityGroupProfile(ir.Nodes, ir.Edges, sourceID)
	if len(srcSG.Unresolved) > 0 {
		step("sg_source_egress", sourceID, "evaluate source's security groups, outbound (union of all attached SGs)", TraceNotAssessable,
			"a security group input could not be read, so the rules are incomplete: "+strings.Join(srcSG.Unresolved, "; "), "")
		return finalizeOutbound(trace)
	}
	if len(srcSG.SGIDs) == 0 {
		step("sg_source_egress", sourceID, "evaluate source's security groups, outbound (union of all attached SGs)", TraceNotAssessable,
			"no security group is attached to "+sourceID+" — its own outbound posture is unknown, never assumed", "")
		return finalizeOutbound(trace)
	}
	openSG := SGProfile{SGIDs: []string{"internet"}, Rules: []SGRule{{Direction: "ingress", Protocol: "-1", CIDRs: []string{"0.0.0.0/0"}}}}
	okConn, initD, _ := EvaluateConnection(srcSG, openSG, srcCIDR, "0.0.0.0/0", protocol, port)
	step("sg_source_egress", sourceID, "evaluate source's security groups, outbound (union of all attached SGs)", sgStepDecision(initD), sgReasonOrDefault(initD), "SG "+initD.MatchedSG)
	if !okConn {
		return finalizeOutbound(trace)
	}

	step("internet_boundary", JourneyInternetSentinel, "the request leaves the design at the internet boundary", TraceAllow,
		"the far side is outside the design; its own network behaviour is not modelled and nothing is claimed about it", "")
	return finalizeOutbound(trace)
}

// PublicAddress is ResolvePublicIPv4's answer: whether a resource holds a public IPv4 address,
// and from which declared fact.
type PublicAddress struct {
	Decision TraceDecision
	Reason   string
	Rule     string
	// Assumed is true when the answer rests on a documented default for an attribute the design
	// does not declare, rather than on a stated value.
	Assumed bool
}

// ResolvePublicIPv4 decides whether sourceID holds a public IPv4 address, from facts the design
// declares, in AWS's own precedence (VPC User Guide, "Enable internet access for a VPC using an
// internet gateway" and "IP addressing"):
//  1. an Elastic IP associated with the resource gives it one, whatever else is set;
//  2. otherwise a launch-time setting on the resource overrides the subnet's attribute;
//  3. otherwise the subnet's auto-assign attribute decides, and when it is not declared the
//     Terraform provider's documented default (false) applies, tagged assumed.
//
// A value declared but not literal (a variable) is unknown, never read as absent. A resource
// type whose mapping gives no public_address_model is not_assessable: how that service obtains a
// public address is not modelled (a Lambda function in a VPC, for one, is never given one by the
// subnet attribute, so applying it would be wrong).
func ResolvePublicIPv4(ir *IR, sourceID, subnetID string) PublicAddress {
	src, ok := findNode(ir, sourceID)
	if !ok {
		return PublicAddress{Decision: TraceNotAssessable, Reason: "source node not found in the IR"}
	}
	if src.RawAttributes["public_address_model"] != "launch_attribute" {
		return PublicAddress{Decision: TraceNotAssessable,
			Reason: "how " + sourceID + " obtains a public IPv4 address is not modelled, so whether it can use an internet gateway is not assessable"}
	}

	for _, e := range ir.Edges {
		if e.From == "" || e.To != sourceID {
			continue
		}
		if n, ok := findNode(ir, e.From); ok && n.RawAttributes["network_role"] == "elastic_ip" {
			return PublicAddress{Decision: TraceAllow, Reason: "an Elastic IP (" + e.From + ") is associated with " + sourceID,
				Rule: "VPC User Guide, Enable internet access for a VPC using an internet gateway"}
		}
	}

	const instanceAttr, subnetAttr = "associate_public_ip_address", "map_public_ip_on_launch"
	if attrUnresolved(src, instanceAttr) {
		return PublicAddress{Decision: TraceNotAssessable, Reason: sourceID + " sets " + instanceAttr + " to a value the design does not resolve, and no Elastic IP is associated"}
	}
	if v, ok := src.RawAttributes[instanceAttr].(bool); ok {
		rule := "VPC User Guide, IP addressing: the launch-time setting overrides the subnet's attribute"
		if v {
			return PublicAddress{Decision: TraceAllow, Reason: sourceID + " is launched with " + instanceAttr + " = true", Rule: rule}
		}
		return PublicAddress{Decision: TraceDeny, Reason: sourceID + " is launched with " + instanceAttr + " = false and no Elastic IP is associated, so it has no public IPv4 address for the internet gateway to translate", Rule: rule}
	}

	sub, ok := findNode(ir, subnetID)
	if !ok {
		return PublicAddress{Decision: TraceNotAssessable, Reason: "subnet " + subnetID + " not found in the IR"}
	}
	if attrUnresolved(sub, subnetAttr) {
		return PublicAddress{Decision: TraceNotAssessable, Reason: subnetID + " sets " + subnetAttr + " to a value the design does not resolve, and " + sourceID + " neither overrides it nor has an Elastic IP"}
	}
	rule := "VPC User Guide, IP addressing: the subnet attribute decides whether a network interface created in it receives a public IPv4 address"
	if v, ok := sub.RawAttributes[subnetAttr].(bool); ok {
		if v {
			return PublicAddress{Decision: TraceAllow, Reason: subnetID + " auto-assigns a public IPv4 address (" + subnetAttr + " = true) and " + sourceID + " does not override it", Rule: rule}
		}
		return PublicAddress{Decision: TraceDeny, Reason: subnetID + " does not auto-assign a public IPv4 address (" + subnetAttr + " = false), " + sourceID + " does not override it, and no Elastic IP is associated", Rule: rule}
	}
	return PublicAddress{Decision: TraceDeny, Assumed: true, Rule: rule,
		Reason: subnetID + " does not declare " + subnetAttr + ", so the Terraform AWS provider's documented default (false) applies; " + sourceID + " does not override it and no Elastic IP is associated"}
}

// attrUnresolved reports whether the node declared attr with a non-literal expression.
func attrUnresolved(n Node, attr string) bool {
	switch names := n.RawAttributes["unresolved_attributes"].(type) {
	case []string:
		for _, x := range names {
			if x == attr {
				return true
			}
		}
	case []any:
		for _, x := range names {
			if s, _ := x.(string); s == attr {
				return true
			}
		}
	}
	return false
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
