// This file is PC-112's final core-level wrapper: real IR nodes (their own
// RawAttributes["security_group_rules"], populated by ingest/securitygroups.go) ->
// core/internal/analyse's real SGRule/SGProfile/EvaluateConnection engine — the same
// "internal/analyse stays pure and provider-agnostic; core/ resolves real IR data into
// it" split PC-111's routing.go already established.
package core

import "preflight/core/internal/analyse"

// SGRule and SGProfile are core's public mirrors of the internal engine's own types —
// I1 (core/internal/* is compiler-enforced private) means the internal shapes aren't
// visible to a caller like tests/aws-conformance/networking, which needs to construct
// profiles directly to exercise the engine as conformance tests.
type SGRule = analyse.SGRule
type SGProfile = analyse.SGProfile
type SGDecision = analyse.SGDecision

// EvaluateConnection is the public entry point to PC-112's real, stateful SG
// evaluation — see core/internal/analyse/securitygroup2.go's own doc comment for the
// full AWS citation and design reasoning.
func EvaluateConnection(initiator, responder SGProfile, initiatorCIDR, responderCIDR, protocol string, port int) (allowed bool, initiatorDecision, responderDecision SGDecision) {
	return analyse.EvaluateConnection(initiator, responder, initiatorCIDR, responderCIDR, protocol, port)
}

// SecurityGroupProfile builds a real SGProfile for one resource from its own attached
// security groups' RawAttributes["security_group_rules"] (ingest/securitygroups.go's
// own output shape) — the SG IDs a resource is attached to are found via its own
// depends_on edges to nodes carrying that key, the same structural-lookup discipline
// core/routing.go's EffectiveRouteTableID already established (a node's ROLE is found
// by what it structurally IS in the graph, never by a provider-specific ID check).
func SecurityGroupProfile(nodes []Node, edges []Edge, resourceID string) SGProfile {
	byID := map[string]Node{}
	for _, n := range nodes {
		byID[n.ID] = n
	}

	var sgIDs []string
	for _, e := range edges {
		if e.Type != EdgeTypeDependsOn || e.From != resourceID {
			continue
		}
		if n, ok := byID[e.To]; ok {
			if _, hasRules := n.RawAttributes["security_group_rules"]; hasRules {
				sgIDs = append(sgIDs, n.ID)
			}
		}
	}

	profile := SGProfile{SGIDs: sgIDs}
	for _, sgID := range sgIDs {
		node := byID[sgID]
		for _, raw := range rawSGRules(node.RawAttributes["security_group_rules"]) {
			profile.Rules = append(profile.Rules, toSGRule(raw))
		}
	}
	return profile
}

// rawSGRules reads the rule list in either of its two real shapes: []map[string]any
// when the IR is still the in-memory value ingest just built, []any of
// map[string]any once it has been through the session store's JSON round trip. A
// type assertion for only the first silently turned every stored SG into "attached,
// zero rules" — an implicit deny the architect never authored (PC-137).
func rawSGRules(v any) []map[string]any {
	switch rules := v.(type) {
	case []map[string]any:
		return rules
	case []any:
		out := make([]map[string]any, 0, len(rules))
		for _, r := range rules {
			if m, ok := r.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

func toSGRule(raw map[string]any) SGRule {
	rule := SGRule{}
	rule.Direction, _ = raw["direction"].(string)
	rule.Protocol, _ = raw["protocol"].(string)
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
	if cidrs, ok := raw["cidr_blocks"].([]any); ok {
		for _, c := range cidrs {
			if s, ok := c.(string); ok {
				rule.CIDRs = append(rule.CIDRs, s)
			}
		}
	} else if cidrs, ok := raw["cidr_blocks"].([]string); ok {
		rule.CIDRs = cidrs
	}
	rule.SourceSG, _ = raw["source_security_group"].(string)
	return rule
}
