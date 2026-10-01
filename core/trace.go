// This file is PC-114: per-request simulation with an explainable, ordered trace,
// combining PC-111 (routes), PC-112 (SG), and PC-113 (NACL) into one pipeline — never
// reimplementing any of their own evaluation logic, only sequencing calls into them.
// Pipeline shape, per the Card's own named reference model: resolve source -> resolve
// destination -> route selection -> NACL (source subnet, outbound) -> NACL (dest
// subnet, inbound) -> SG (dest ENI inbound) -> target health -> response path (NACL
// return rules; SG statefulness is automatic and produces no separate step, per
// PC-112's own engine).
//
// CAPABILITY GATE: PC-107 landed after this file was first written. The gate below
// now reads a node's REAL, per-service capability_level (RawAttributes["capability_level"],
// stamped by ingest/build.go from providers/*'s own registry entry — see
// core/capability.go) rather than a static NodeType whitelist. A node with no
// resolvable capability_level (a hand-built canvas/test IR that never went through a
// registry, or a genuinely unmapped service) is honestly not_assessable, never
// guessed capable.
package core

import "fmt"

type TraceDecision string

const (
	TraceAllow        TraceDecision = "allow"
	TraceDeny         TraceDecision = "deny"
	TraceNotAssessable TraceDecision = "not_assessable"
)

// TraceStep is one provenance-tagged step of a request trace — PC-114's own
// acceptance criterion, verbatim: "Each step is a provenance-tagged record: step,
// component, operation, decision (allow / deny / not_assessable), reason, AWS rule
// citation."
type TraceStep struct {
	Step       string        `json:"step"`
	Component  string        `json:"component"`
	Operation  string        `json:"operation"`
	Decision   TraceDecision `json:"decision"`
	Reason     string        `json:"reason"`
	RuleCited  string        `json:"rule_cited,omitempty"`
	Reached    bool          `json:"reached"`
	Provenance Provenance    `json:"provenance"`
}

// Trace is the full result — both presentations the Card asks for share this one
// underlying structure ("Two presentations of the same trace ... Same data, not two
// computations"): Concise is derived FROM Steps, not computed separately.
type Trace struct {
	Source      string      `json:"source"`
	Destination string      `json:"destination"`
	Protocol    string      `json:"protocol"`
	Port        int         `json:"port"`
	Allowed     bool        `json:"allowed"`
	Concise     string      `json:"concise"`
	Steps       []TraceStep `json:"steps"`
}

// requestSimulationCapable reads a node's real, registry-declared capability_level
// (PC-107) and reports whether it reaches CapabilityRequestSimulation. A node with no
// resolvable capability_level (never ingested through a registry mapping at all) is
// honestly reported not capable, never guessed either way — see this file's own
// package doc comment.
func requestSimulationCapable(n Node) bool {
	raw, ok := n.RawAttributes["capability_level"].(string)
	if !ok {
		return false
	}
	return CapabilityLevel(raw).AtLeast(CapabilityRequestSimulation)
}

// resolveSubnetID finds the real subnet a resource is placed in by walking its own
// contained_in chain (a resource may sit directly in a subnet, or indirectly via a
// subnet-group indirection — PC-80's own db_subnet_group/elasticache_subnet_group
// pattern) and returning the first node in that chain that is itself structurally a
// subnet — identified the same way EffectiveRouteTableID already does (a node with a
// real, resolvable effective route table), never by a provider-specific ID check.
func resolveSubnetID(ir *IR, resourceID string) (string, bool) {
	visited := map[string]bool{}
	queue := []string{resourceID}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if visited[id] {
			continue
		}
		visited[id] = true
		if id != resourceID {
			if _, ok := EffectiveRouteTableID(ir.Edges, id); ok {
				return id, true
			}
		}
		for _, e := range ir.Edges {
			if e.Type == EdgeTypeContainedIn && e.From == id {
				queue = append(queue, e.To)
			}
		}
	}
	return "", false
}

// subnetCIDR returns a subnet node's own declared cidr_block — ingest/build.go's
// buildNode captures every literal Terraform attribute onto RawAttributes
// unconditionally, aws_subnet included (providers/aws/subnet.yaml's own doc comment),
// so this is real per-resource data, not a guess. Used as the traffic-address stand-in
// for a resource whose own individual IP the IR does not model at all (there is no
// per-resource IP field) — the closest real fact available is which subnet, and
// therefore which CIDR range, it lives in.
func subnetCIDR(ir *IR, subnetID string) (string, bool) {
	n, ok := findNode(ir, subnetID)
	if !ok {
		return "", false
	}
	cidr, ok := n.RawAttributes["cidr_block"].(string)
	if !ok || cidr == "" {
		return "", false
	}
	return cidr, true
}

func findNode(ir *IR, id string) (Node, bool) {
	for _, n := range ir.Nodes {
		if n.ID == id {
			return n, true
		}
	}
	return Node{}, false
}

// BuildTrace runs the real, ordered pipeline. sourceID/destID are IR node IDs — PC-114's
// own example: "internet" is not a real IR node (there is no modeled "internet" node),
// so an internet-originated request is represented by passing an empty sourceID with a
// sourceCIDR of 0.0.0.0/0 instead; a resource-to-resource request passes a real
// sourceID and an empty sourceCIDR.
func BuildTrace(ir *IR, sourceID, destID, sourceCIDR, protocol string, port int) Trace {
	prov := NewProvenance(KindDerived, fmt.Sprintf("core/trace:%s->%s:%s/%d", sourceID, destID, protocol, port))
	trace := Trace{Source: sourceID, Destination: destID, Protocol: protocol, Port: port}

	reached := true
	step := func(name, component, operation string, decision TraceDecision, reason, rule string) TraceStep {
		s := TraceStep{Step: name, Component: component, Operation: operation, Decision: decision, Reason: reason, RuleCited: rule, Reached: reached, Provenance: prov}
		trace.Steps = append(trace.Steps, s)
		if reached && decision != TraceAllow {
			reached = false
		}
		return s
	}

	// 1. resolve_destination (the one side that must be a real IR node)
	destNode, destOK := findNode(ir, destID)
	if !destOK {
		step("resolve_destination", destID, "look up the destination node in the IR", TraceDeny, "destination node not found in the IR", "")
		return finalize(trace)
	}
	step("resolve_destination", destID, "look up the destination node in the IR", TraceAllow, "destination node exists", "")

	// 2. resolve_source, only when a real source node ID was given (not an internet/CIDR-only source)
	var sourceNode Node
	if sourceID != "" {
		var ok bool
		sourceNode, ok = findNode(ir, sourceID)
		if !ok {
			step("resolve_source", sourceID, "look up the source node in the IR", TraceDeny, "source node not found in the IR", "")
			return finalize(trace)
		}
		step("resolve_source", sourceID, "look up the source node in the IR", TraceAllow, "source node exists", "")
	}

	// 3. capability gate (PC-107: real per-service capability_level, not a NodeType whitelist)
	if !requestSimulationCapable(destNode) {
		step("capability_check", destID, "check whether this service supports request simulation", TraceNotAssessable,
			fmt.Sprintf("service %s (capability_level %v) does not reach REQUEST_SIMULATION", destID, destNode.RawAttributes["capability_level"]), "")
		return finalize(trace)
	}
	if sourceID != "" && !requestSimulationCapable(sourceNode) {
		step("capability_check", sourceID, "check whether this service supports request simulation", TraceNotAssessable,
			fmt.Sprintf("service %s (capability_level %v) does not reach REQUEST_SIMULATION", sourceID, sourceNode.RawAttributes["capability_level"]), "")
		return finalize(trace)
	}
	step("capability_check", destID, "check whether this service supports request simulation", TraceAllow, "service type is modelled for request simulation", "")

	destSubnetID, destHasSubnet := resolveSubnetID(ir, destID)
	var sourceSubnetID string
	sourceHasSubnet := false
	if sourceID != "" {
		sourceSubnetID, sourceHasSubnet = resolveSubnetID(ir, sourceID)
	}

	// 4. route selection (only meaningful resource-to-resource, and only when we know
	// both subnets — an internet-originated request has no source subnet to route
	// from, so this step is skipped for it, not guessed).
	if sourceID != "" {
		if !destHasSubnet {
			step("route_selection", destID, "resolve the destination's effective route table", TraceNotAssessable, "destination has no resolvable effective route table", "")
			return finalize(trace)
		}
		if !sourceHasSubnet {
			step("route_selection", sourceID, "resolve the source's effective route table", TraceNotAssessable, "source has no resolvable effective route table", "")
			return finalize(trace)
		}
		if sourceSubnetID != destSubnetID {
			step("route_selection", sourceSubnetID, "select a route from source to destination subnet", TraceAllow, "source and destination are in different subnets; routing applies (NACLs evaluated next)", "")
		} else {
			step("route_selection", sourceSubnetID, "select a route from source to destination subnet", TraceAllow, "source and destination are in the same subnet — traffic does not cross a route table or NACL", "")
		}
	}

	// 5/6. NACL at each subnet boundary — skipped entirely for same-subnet traffic
	// (EvaluateNACLPath's own same-subnet short-circuit). An internet-originated
	// request has no source subnet of its own, but it still crosses the destination
	// subnet's NACL on the way in — represented with an all-permissive placeholder
	// profile standing in for "the internet has no NACL of its own to check", the same
	// device used for the SG step below, so only the real destination NACL constrains
	// the outcome.
	naclApplies := destHasSubnet && (sourceID == "" || (sourceHasSubnet && sourceSubnetID != destSubnetID))
	if naclApplies {
		destNACL, destHasNACL := NACLProfileForSubnet(ir.Nodes, ir.Edges, destSubnetID)
		if !destHasNACL {
			step("nacl_check", destSubnetID, "resolve the associated NACL for the destination subnet", TraceNotAssessable, "destination subnet has no resolvable associated NACL", "")
			return finalize(trace)
		}
		sourceNACL := NACLProfile{NACLID: "internet", Rules: []NACLRule{
			{Number: NACLCatchAll, Direction: "ingress", Protocol: "-1", CIDR: "0.0.0.0/0", Allow: true},
			{Number: NACLCatchAll, Direction: "egress", Protocol: "-1", CIDR: "0.0.0.0/0", Allow: true},
		}}
		sourceSubnetLabel := "internet"
		if sourceID != "" {
			var sourceHasNACL bool
			sourceNACL, sourceHasNACL = NACLProfileForSubnet(ir.Nodes, ir.Edges, sourceSubnetID)
			if !sourceHasNACL {
				step("nacl_check", sourceSubnetID, "resolve the associated NACL for the source subnet", TraceNotAssessable, "source subnet has no resolvable associated NACL", "")
				return finalize(trace)
			}
			sourceSubnetLabel = sourceSubnetID
		}
		// No per-resource IP is modelled in the IR, so the closest real fact standing
		// in for each side's address is its own subnet's declared cidr_block; only
		// when even that isn't resolvable does this fall back to "0.0.0.0/0" meaning
		// "this address is genuinely unknown" — cidrMatch then reports a
		// CIDR-restricted rule as ambiguous (not_assessable) rather than silently
		// guessing allow or deny.
		effSourceCIDR := sourceCIDR
		if effSourceCIDR == "" {
			if c, ok := subnetCIDR(ir, sourceSubnetLabel); ok {
				effSourceCIDR = c
			} else {
				effSourceCIDR = "0.0.0.0/0"
			}
		}
		effDestCIDR := "0.0.0.0/0"
		if c, ok := subnetCIDR(ir, destSubnetID); ok {
			effDestCIDR = c
		}
		_, allowed, fwd, ret := EvaluateNACLPath(sourceSubnetLabel, destSubnetID, sourceNACL, destNACL, effSourceCIDR, effDestCIDR, protocol, port, 0, 0)
		d := TraceDeny
		if allowed {
			d = TraceAllow
		}
		step("nacl_source_egress", sourceNACL.NACLID, "evaluate source subnet's NACL, outbound", boolDecision(fwd[0].Allowed), fwd[0].Reason, "NACL "+fwd[0].NACLID+" rule "+fwd[0].MatchedRuleNumber)
		if fwd[0].Allowed {
			step("nacl_dest_ingress", destNACL.NACLID, "evaluate destination subnet's NACL, inbound", boolDecision(fwd[1].Allowed), fwd[1].Reason, "NACL "+fwd[1].NACLID+" rule "+fwd[1].MatchedRuleNumber)
		}
		if !allowed && fwd[0].Allowed && fwd[1].Allowed {
			// forward legs both passed; the failure is on the return leg
			step("nacl_response_path", destNACL.NACLID, "evaluate the stateless return path (destination egress, source ingress, ephemeral ports)", d, "return traffic denied: "+ret[0].Reason+"; "+ret[1].Reason, "")
			return finalize(trace)
		}
		if !allowed {
			return finalize(trace)
		}
	}
	// 7. SG at the destination ENI (inbound) — the one step every request always gets,
	// resource-to-resource or internet-originated.
	destSG := SecurityGroupProfile(ir.Nodes, ir.Edges, destID)
	var sourceSG SGProfile
	if sourceID != "" {
		sourceSG = SecurityGroupProfile(ir.Nodes, ir.Edges, sourceID)
	} else {
		sourceSG = SGProfile{Rules: []SGRule{{Direction: "egress", Protocol: "-1", CIDRs: []string{"0.0.0.0/0"}}}}
	}
	// PC-137: a resource with genuinely ZERO attached security groups (no
	// depends_on edge to any node carrying security_group_rules at all) is a
	// missing-data case, not a real "no rule matched" implicit deny — I4 applies
	// here exactly as it already does at nacl_check above ("destination subnet has
	// no resolvable associated NACL" → not_assessable, never a guessed deny). A real
	// SG that IS attached but whose own authored rule list happens to be empty (or
	// simply doesn't match) is a genuine, computed implicit deny — AWS's own
	// documented "allow rules only" semantics — and stays exactly that, unaffected
	// by this check.
	if len(destSG.SGIDs) == 0 || (sourceID != "" && len(sourceSG.SGIDs) == 0) {
		unresolved := destID
		if len(destSG.SGIDs) > 0 {
			unresolved = sourceID
		}
		step("sg_dest_ingress", unresolved, "evaluate destination's security groups, inbound (union of all attached SGs)", TraceNotAssessable,
			"no security group is attached to "+unresolved+" — its own inbound/outbound posture is unknown, never assumed", "")
		return finalize(trace)
	}
	// Same "subnet CIDR stands in for the unmodelled per-resource IP" reasoning as the
	// NACL step above.
	effectiveSourceCIDR := sourceCIDR
	if effectiveSourceCIDR == "" {
		if c, ok := subnetCIDR(ir, sourceSubnetID); sourceHasSubnet && ok {
			effectiveSourceCIDR = c
		} else {
			effectiveSourceCIDR = "0.0.0.0/0"
		}
	}
	effectiveDestCIDR := "0.0.0.0/0"
	if destHasSubnet {
		if c, ok := subnetCIDR(ir, destSubnetID); ok {
			effectiveDestCIDR = c
		}
	}
	allowedConn, initD, respD := EvaluateConnection(sourceSG, destSG, effectiveSourceCIDR, effectiveDestCIDR, protocol, port)
	sgDecision := respD
	if !initD.Allowed {
		sgDecision = initD
	}
	step("sg_dest_ingress", destID, "evaluate destination's security groups, inbound (union of all attached SGs)", boolDecision(allowedConn), sgReasonOrDefault(sgDecision), "SG "+sgDecision.MatchedSG)
	if !allowedConn {
		return finalize(trace)
	}

	// 7b. LB target registration (PC-130): when the source is a load balancer whose
	// targets are modelled (canvas routes_to edges), the destination must still be one
	// of them. No modelled registration (HCL bundles) = no claim, never a guessed deny.
	if sourceID != "" && sourceNode.Type == NodeTypeLoadBalancer {
		if known, registered := targetRegistration(ir, sourceID, destID); known {
			if registered {
				step("target_registration", destID, "check the destination is a registered target of the load balancer", TraceAllow, destID+" is registered with "+sourceID, "")
			} else {
				step("target_registration", destID, "check the destination is a registered target of the load balancer", TraceDeny, destID+" is not a registered target of "+sourceID+" (deregistered or never registered)", "LB "+sourceID+" targets")
				return finalize(trace)
			}
		}
	}

	// 8. target health — structural completeness only (no live account access, P0):
	// the target resolved to a real, known (not unresolved) node.
	healthDecision := TraceAllow
	healthReason := "destination node is fully resolved"
	if destNode.Resolution != ResolutionKnown {
		healthDecision = TraceNotAssessable
		healthReason = "destination node's own resolution state is " + string(destNode.Resolution) + ", not known — health cannot be assessed"
	}
	step("target_health", destID, "check the destination node's own resolution state (structural completeness, not a live health check — P0 is static analysis only)", healthDecision, healthReason, "")

	return finalize(trace)
}

func boolDecision(allowed bool) TraceDecision {
	if allowed {
		return TraceAllow
	}
	return TraceDeny
}

func sgReasonOrDefault(d SGDecision) string {
	if d.Reason != "" {
		return d.Reason
	}
	return "no reason recorded"
}

// finalize computes Allowed and the concise summary FROM Steps — the same
// underlying data both presentations use, per the Card's own instruction.
func finalize(t Trace) Trace {
	t.Allowed = true
	var failing *TraceStep
	for i := range t.Steps {
		if t.Steps[i].Decision != TraceAllow {
			t.Allowed = false
			failing = &t.Steps[i]
			break
		}
	}
	if failing == nil {
		t.Concise = fmt.Sprintf("Allowed: %s -> %s:%d/%s reaches its destination.", labelOr(t.Source, "internet"), t.Destination, t.Port, t.Protocol)
	} else {
		t.Concise = fmt.Sprintf("Blocked at %s (%s): %s", failing.Component, failing.Step, failing.Reason)
	}
	return t
}

func labelOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
