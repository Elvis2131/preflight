// This file is PC-113's core-level wrapper around core/internal/analyse's real NACL
// engine (nacl.go) — same "internal/analyse stays pure; core/ resolves real IR data
// into it" split PC-111/112 already established.
package core

import "preflight/core/internal/analyse"

type NACLRule = analyse.NACLRule
type NACLProfile = analyse.NACLProfile
type NACLDecision = analyse.NACLDecision

const NACLCatchAll = analyse.NACLCatchAll

// EvaluateNACLDirectional is the public entry point to the same-named internal
// function, for callers (conformance tests) that already have a profile in hand.
func EvaluateNACLDirectional(profile NACLProfile, direction, cidr, protocol string, port int) NACLDecision {
	return analyse.EvaluateNACLDirectional(profile, direction, cidr, protocol, port)
}

// EvaluateNACLPath is PC-113's own acceptance criterion, verbatim: "Evaluated at
// subnet boundaries, so traffic between resources in the same subnet does not cross a
// NACL" — cited against AWS's own "We evaluate the network ACL rules when traffic
// enters and leaves the subnet, not as it is routed within a subnet."
// (docs.aws.amazon.com/vpc/latest/userguide/vpc-network-acls.html). When sourceSubnetID
// equals destSubnetID, this function returns immediately without evaluating either
// NACL at all — same-subnet traffic never crosses one, so there is nothing to decide,
// not an "allowed" verdict this engine invented.
func EvaluateNACLPath(sourceSubnetID, destSubnetID string, source, dest NACLProfile, sourceCIDR, destCIDR, protocol string, port, ephemeralFrom, ephemeralTo int) (evaluated, allowed bool, forward, returnLeg [2]NACLDecision) {
	if sourceSubnetID == destSubnetID {
		return false, true, [2]NACLDecision{}, [2]NACLDecision{}
	}
	allowed, forward, returnLeg = analyse.EvaluateNACLConnection(source, dest, sourceCIDR, destCIDR, protocol, port, ephemeralFrom, ephemeralTo)
	return true, allowed, forward, returnLeg
}

// NACLProfileForSubnet builds a real NACLProfile for a subnet from its own attached
// NACL node's RawAttributes["nacl_rules"] (ingest/nacl.go's own output shape) — the
// subnet's associated NACL is found via its own depends_on edge to a node carrying
// that key, the same structural-lookup discipline EffectiveRouteTableID/
// SecurityGroupProfile already established (a node's ROLE is found by what it
// structurally IS in the graph, never a provider-specific ID check).
func NACLProfileForSubnet(nodes []Node, edges []Edge, subnetID string) (NACLProfile, bool) {
	byID := map[string]Node{}
	for _, n := range nodes {
		byID[n.ID] = n
	}
	for _, e := range edges {
		if e.Type != EdgeTypeDependsOn || e.From != subnetID {
			continue
		}
		n, ok := byID[e.To]
		if !ok {
			continue
		}
		if !hasRawRules(n.RawAttributes["nacl_rules"]) {
			continue
		}
		rawRules := rawRuleMaps(n.RawAttributes["nacl_rules"])
		profile := NACLProfile{NACLID: n.ID}
		for _, raw := range rawRules {
			profile.Rules = append(profile.Rules, toNACLRule(raw))
		}
		return profile, true
	}
	return NACLProfile{}, false
}

func toNACLRule(raw map[string]any) NACLRule {
	rule := NACLRule{}
	rule.Direction, _ = raw["direction"].(string)
	rule.Protocol, _ = raw["protocol"].(string)
	rule.CIDR, _ = raw["cidr"].(string)
	rule.Allow, _ = raw["allow"].(bool)
	if n, ok := raw["number"].(float64); ok {
		rule.Number = int(n)
	} else if n, ok := raw["number"].(int); ok {
		rule.Number = n
	} else {
		rule.Number = NACLCatchAll
	}
	if v, ok := raw["from_port"].(float64); ok {
		rule.FromPort = int(v)
	} else if v, ok := raw["from_port"].(int); ok {
		rule.FromPort = v
	}
	if v, ok := raw["to_port"].(float64); ok {
		rule.ToPort = int(v)
	} else if v, ok := raw["to_port"].(int); ok {
		rule.ToPort = v
	}
	return rule
}

// NACLSource says where a subnet's effective NACL came from (PC-149).
type NACLSource string

const (
	// NACLExplicit: the subnet is associated with a NACL in the design.
	NACLExplicit NACLSource = "explicit"
	// NACLDeclaredDefault: no association, and the design declares the VPC default
	// NACL's rules (Terraform aws_default_network_acl).
	NACLDeclaredDefault NACLSource = "declared_default"
	// NACLAssumedDefault: no association and nothing declared — AWS's documented
	// default NACL, assumed unmodified. The ONE assumption is NACLAssumedDefaultReason.
	NACLAssumedDefault NACLSource = "assumed_default"
)

// NACLAssumedDefaultID is the profile ID of the assumed default NACL.
const NACLAssumedDefaultID = "default-nacl (assumed)"

// NACLAssumedDefaultReason states the single real assumption, for the trace step.
const NACLAssumedDefaultReason = "no NACL is associated with this subnet and the design declares no default NACL rules, so AWS's documented default network ACL applies (VPC User Guide, \"Default network ACL for a VPC\": rule 100 allows all inbound and outbound traffic, plus the * deny); ASSUMED unmodified — the design cannot show that nobody changed the default NACL outside it"

// NACLResolution is a subnet's effective NACL and its provenance.
type NACLResolution struct {
	Profile NACLProfile
	Source  NACLSource
}

// ResolveSubnetNACL applies AWS's association rule (VPC User Guide, "Control subnet
// traffic with network access control lists": "If you don't explicitly associate a
// subnet with a network ACL, the subnet is automatically associated with the default
// network ACL"):
//  1. an explicit association -> that NACL;
//  2. none, and the design declares the subnet's VPC default NACL -> those rules;
//  3. none, and nothing declared -> AWS's documented allow-all default, tagged assumed.
//
// ok is false (not_assessable, I4) when an explicit association cannot be ruled out —
// an unresolved or dangling depends_on edge from the subnet — or when default NACLs
// are declared but this subnet's VPC cannot be resolved to choose between them.
func ResolveSubnetNACL(nodes []Node, edges []Edge, subnetID string) (NACLResolution, bool) {
	// PC-158: an association or a rule ingest could not read puts this subnet's NACL in doubt.
	for _, n := range nodes {
		if n.ID == subnetID && len(UnresolvedInputs(n, AffectsNACL)) > 0 {
			return NACLResolution{}, false
		}
	}
	if p, ok := NACLProfileForSubnet(nodes, edges, subnetID); ok {
		for _, n := range nodes {
			if n.ID == p.NACLID && len(UnresolvedInputs(n, AffectsNACL)) > 0 {
				return NACLResolution{}, false // a rule or association ingest could not read: the profile is incomplete
			}
		}
		return NACLResolution{Profile: p, Source: NACLExplicit}, true
	}
	byID := map[string]Node{}
	for _, n := range nodes {
		byID[n.ID] = n
	}
	for _, e := range edges {
		if e.Type == EdgeTypeDependsOn && e.From == subnetID {
			if _, ok := byID[e.To]; !ok || e.Resolution != ResolutionKnown {
				return NACLResolution{}, false
			}
		}
	}

	var vpcID string
	for _, e := range edges {
		if e.Type == EdgeTypeContainedIn && e.From == subnetID && e.Resolution == ResolutionKnown {
			if _, ok := byID[e.To]; ok {
				vpcID = e.To
				break
			}
		}
	}
	declaredAny := false
	for _, n := range nodes {
		if d, _ := n.RawAttributes["default_nacl"].(bool); !d || !hasRawRules(n.RawAttributes["nacl_rules"]) {
			continue
		}
		declaredAny = true
		if len(UnresolvedInputs(n, AffectsNACL)) > 0 {
			return NACLResolution{}, false // the declared default has a rule ingest could not read
		}
		for _, e := range edges {
			if e.Type == EdgeTypeContainedIn && e.From == n.ID && e.To == vpcID && vpcID != "" {
				profile := NACLProfile{NACLID: n.ID}
				for _, raw := range rawRuleMaps(n.RawAttributes["nacl_rules"]) {
					profile.Rules = append(profile.Rules, toNACLRule(raw))
				}
				return NACLResolution{Profile: profile, Source: NACLDeclaredDefault}, true
			}
		}
	}
	if declaredAny && vpcID == "" {
		return NACLResolution{}, false
	}
	return NACLResolution{Profile: NACLProfile{NACLID: NACLAssumedDefaultID, Rules: []NACLRule{
		{Number: 100, Direction: "ingress", Protocol: "-1", CIDR: "0.0.0.0/0", Allow: true},
		{Number: 100, Direction: "egress", Protocol: "-1", CIDR: "0.0.0.0/0", Allow: true},
	}}, Source: NACLAssumedDefault}, true
}
