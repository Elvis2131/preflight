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
