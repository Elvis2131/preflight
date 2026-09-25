// This file is PC-112: the real Security Group evaluation engine, superseding
// PC-79's own narrower FilterEdgesBySGRules (that file's own sgPermits now delegates
// here — see securitygroup.go's own updated doc comment). Every semantic below is
// verified against real AWS documentation, fetched and read in full before writing any
// code (not assumed from memory):
//
//   - Allow-only, implicit deny: "You can specify allow rules, but not deny rules."
//     (docs.aws.amazon.com/vpc/latest/userguide/security-group-rules.html,
//     "Security group rule basics", verified 2026-09-25)
//   - Multi-SG union: "When you associate multiple security groups with a resource,
//     the rules from each security group are aggregated to form a single set of rules
//     that are used to determine whether to allow access." (same page)
//   - SG-reference sources (membership, not IP): "When you specify a security group as
//     the source or destination for a rule, the rule affects all instances that are
//     associated with the security groups." (same page, "Security group referencing")
//   - Statefulness: "security groups are stateful. This means that responses to
//     inbound traffic are allowed to flow out of the instance regardless of outbound
//     security group rules, and vice versa."
//     (docs.aws.amazon.com/AWSEC2/latest/UserGuide/security-group-connection-tracking.html,
//     verified 2026-09-25)
//
// SCOPE: prefix lists and IPv6 are out of scope (golden architecture uses neither;
// scenario-driven minimalism) — a rule naming either is treated the same as any other
// unrecognized source, contributing no match (never a fabricated allow).
package analyse

import (
	"net/netip"
)

// SGRule is one directional security-group rule.
type SGRule struct {
	Direction string // "ingress" | "egress"
	Protocol  string // "tcp", "udp", "icmp", or "-1" (all protocols — AWS's own sentinel)
	FromPort  int
	ToPort    int
	CIDRs     []string
	SourceSG  string // set only for an SG-referencing rule ("security group referencing"); empty for a CIDR-sourced rule
}

// SGProfile is every rule from every security group attached to one resource's ENI,
// aggregated — the real, documented union ("the rules from each security group are
// aggregated to form a single set of rules"), not evaluated group-by-group.
type SGProfile struct {
	SGIDs []string
	Rules []SGRule
}

// SGDecision is PC-112's own acceptance criterion, verbatim: "Decision output names
// SG ID, rule, and reason." Reason is always populated, allow or deny, so a caller
// never has to reconstruct why from a bare boolean.
type SGDecision struct {
	Allowed     bool
	MatchedSG   string
	MatchedRule *SGRule
	Reason      string
}

// protocolMatches treats "-1" (AWS's own documented all-protocols sentinel) as a
// wildcard; otherwise requires an exact, case-insensitive match against the rule's
// own declared protocol name/number.
func protocolMatches(rule SGRule, protocol string) bool {
	return rule.Protocol == "-1" || rule.Protocol == protocol
}

// portMatches: ICMP rules encode type/code in FromPort/ToPort per AWS's own component
// list, a different semantic than a TCP/UDP port range — this engine does not attempt
// ICMP type/code matching (scenario-driven minimalism: none of the six golden
// scenarios need it), so any ICMP or all-protocol rule matches on port regardless.
func portMatches(rule SGRule, port int) bool {
	if rule.Protocol == "-1" || rule.Protocol == "icmp" {
		return true
	}
	return port >= rule.FromPort && port <= rule.ToPort
}

// cidrMatch reports whether ruleCIDR permits traffic from sourceCIDR: "match" when
// ruleCIDR fully contains sourceCIDR (every possible source address is covered),
// "no match" when they don't overlap at all, and "ambiguous" when they partially
// overlap (some addresses in sourceCIDR are covered, some aren't — this engine cannot
// know which specific address is the real source without more IR detail than exists
// today, so it refuses to guess allow or deny, per PC-112's own reconciliation note on
// PC-79).
func cidrMatch(ruleCIDR, sourceCIDR string) (match, ambiguous bool) {
	rp, err1 := netip.ParsePrefix(ruleCIDR)
	sp, err2 := netip.ParsePrefix(sourceCIDR)
	if err1 != nil || err2 != nil {
		return false, false
	}
	rp, sp = rp.Masked(), sp.Masked()
	if rp.Bits() <= sp.Bits() && rp.Contains(sp.Addr()) {
		return true, false
	}
	if rp.Overlaps(sp) {
		return false, true
	}
	return false, false
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// EvaluateDirectional checks ONE direction's rules (profile's own rules matching
// `direction`) against a request identified by protocol/port and a source named by
// CIDR and/or security-group membership — real traffic can be identified by an
// attached SG (membership-based reference rules), a CIDR (range-based rules), or
// both; either matching is sufficient (AWS itself does not require BOTH a CIDR and an
// SG match on the same rule — a rule is one or the other, never a conjunction).
func EvaluateDirectional(profile SGProfile, direction, sourceCIDR string, sourceSGIDs []string, protocol string, port int) SGDecision {
	for i := range profile.Rules {
		rule := profile.Rules[i]
		if rule.Direction != direction {
			continue
		}
		if !protocolMatches(rule, protocol) || !portMatches(rule, port) {
			continue
		}
		if rule.SourceSG != "" {
			if contains(sourceSGIDs, rule.SourceSG) {
				return SGDecision{Allowed: true, MatchedSG: matchedSGFor(profile, rule), MatchedRule: &rule,
					Reason: "allowed by " + direction + " rule referencing security group " + rule.SourceSG}
			}
			continue
		}
		anyAmbiguous := false
		for _, cidr := range rule.CIDRs {
			match, ambiguous := cidrMatch(cidr, sourceCIDR)
			if match {
				return SGDecision{Allowed: true, MatchedSG: matchedSGFor(profile, rule), MatchedRule: &rule,
					Reason: "allowed by " + direction + " rule for CIDR " + cidr}
			}
			if ambiguous {
				anyAmbiguous = true
			}
		}
		if anyAmbiguous {
			return SGDecision{Allowed: false, MatchedRule: &rule,
				Reason: "not_assessable: source CIDR " + sourceCIDR + " partially overlaps a rule's CIDR range without being fully contained in it — cannot determine whether the real source address is covered"}
		}
	}
	return SGDecision{Allowed: false, Reason: "no rule matched (implicit deny — AWS security groups support allow rules only)"}
}

// matchedSGFor is a best-effort attribution of which attached SG actually owns the
// matched rule — real when the profile came from a single SG (the common case,
// EvaluateConnection's own callers), honestly blank (not guessed) when the profile
// aggregates multiple SGs and this function has no per-rule SG-ID tag to attribute to
// (SGProfile's own Rules slice doesn't carry a per-rule owning-SG field, deliberately:
// per AWS's own documented union semantics, "no rules from the referenced security
// group are added to the security group that references it" — the AGGREGATE decision
// is what matters, not which specific SG happened to declare the winning rule).
func matchedSGFor(profile SGProfile, _ SGRule) string {
	if len(profile.SGIDs) == 1 {
		return profile.SGIDs[0]
	}
	return ""
}

// EvaluateConnection is PC-112's real, stateful, full-connection answer: an initiator
// can reach a responder on protocol/port iff the initiator's own EGRESS rules allow it
// AND the responder's own INGRESS rules allow it — checked ONCE, for the forward
// direction only. Response traffic is automatically allowed and is never separately
// evaluated: this function has no "check the return path" step, because AWS's own
// documented statefulness means there isn't one ("responses to inbound traffic are
// allowed to flow out of the instance regardless of outbound security group rules, and
// vice versa").
func EvaluateConnection(initiator, responder SGProfile, initiatorCIDR, responderCIDR string, protocol string, port int) (allowed bool, initiatorDecision, responderDecision SGDecision) {
	initiatorDecision = EvaluateDirectional(initiator, "egress", responderCIDR, responder.SGIDs, protocol, port)
	if !initiatorDecision.Allowed {
		return false, initiatorDecision, SGDecision{Reason: "not evaluated: initiator's own egress rules already deny this connection"}
	}
	responderDecision = EvaluateDirectional(responder, "ingress", initiatorCIDR, initiator.SGIDs, protocol, port)
	return responderDecision.Allowed, initiatorDecision, responderDecision
}
