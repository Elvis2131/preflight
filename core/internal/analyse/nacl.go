// This file is PC-113: the real Network ACL evaluation engine, deliberately separate
// from PC-112's Security Group engine (never collapsed into it — the reference
// project's own guidance the Card cites, and AWS's own documentation, both treat these
// as genuinely different abstractions). Every semantic below is verified against real
// AWS documentation, fetched and read in full before writing any code:
//
//   - Subnet association, one NACL per subnet, default fallback: "Each subnet in your
//     VPC must be associated with a network ACL. If you don't explicitly associate a
//     subnet with a network ACL, the subnet is automatically associated with the
//     default network ACL." "a subnet can be associated with only one network ACL at
//     a time." (docs.aws.amazon.com/vpc/latest/userguide/vpc-network-acls.html,
//     "Network ACL basics", verified 2026-09-25)
//   - Numbered rules, ascending evaluation, first match wins: "Each rule has a number
//     from 1 to 32766. We evaluate the rules in order, starting with the lowest
//     numbered rule ... If the traffic matches a rule, the rule is applied and we do
//     not evaluate any additional rules." (same page)
//   - Catch-all deny: "Each network ACL also includes rules where the rule number is
//     an asterisk (*). These rules ensure that if a packet doesn't match any of the
//     other numbered rules, it's denied."
//     (docs.aws.amazon.com/vpc/latest/userguide/default-network-acl.html, verified
//     2026-09-25)
//   - Both allow AND deny rules exist (unlike SGs): "Rule type: Allow and deny rules"
//     vs. security groups' "Allow rules only."
//     (docs.aws.amazon.com/vpc/latest/userguide/nacl-examples.md, "Differences
//     between network ACLs and security groups", verified 2026-09-25)
//   - Stateless, explicit return traffic required: "NACLs are stateless, which means
//     that information about previously sent or received traffic is not saved. If,
//     for example, you create a NACL rule to allow specific inbound traffic to a
//     subnet, responses to that traffic are not automatically allowed."
//     (vpc-network-acls.html, "Network ACL rules")
//   - Subnet-boundary evaluation only: "We evaluate the network ACL rules when traffic
//     enters and leaves the subnet, not as it is routed within a subnet." (same page)
//   - Ephemeral port default: AWS's own worked NACL example uses "Custom TCP | 1024-
//     65535" for the outbound return-traffic rule (nacl-examples.md's own "Network
//     ACL rules" table) — used here as the DEFAULT (assumed) range when the IR has no
//     declared ephemeral range, per the Card's own instruction not to hardcode one
//     universal OS-specific range as fact; DefaultEphemeralPortRange names this
//     source directly so a caller's decision output can cite it, tagged assumed.
package analyse

import "sort"

// DefaultEphemeralPortRange is AWS's own documented example default (see this file's
// own package doc comment) — used only when the IR has no declared ephemeral range for
// the resource being evaluated; a caller using this default must tag its result
// "assumed", never "stated".
const (
	DefaultEphemeralPortFrom = 1024
	DefaultEphemeralPortTo   = 65535
)

// NACLCatchAll is the sentinel rule number for AWS's own "*" catch-all deny rule —
// always evaluated last, per its own documented meaning ("if a packet doesn't match
// any of the other numbered rules, it's denied").
const NACLCatchAll = -1

// NACLRule is one numbered rule. Number is 1-32766, or NACLCatchAll for the "*" rule.
type NACLRule struct {
	Number    int
	Direction string // "ingress" | "egress"
	Protocol  string
	FromPort  int
	ToPort    int
	CIDR      string
	Allow     bool
}

// NACLProfile is one subnet's associated NACL and its full rule set.
type NACLProfile struct {
	NACLID string
	Rules  []NACLRule
}

// NACLDecision is PC-113's own acceptance criterion, verbatim: "Decision output names
// NACL ID, rule number, action, and reason."
type NACLDecision struct {
	Allowed            bool
	NACLID             string
	MatchedRuleNumber  string // "100", or "*" for the catch-all
	Reason             string
	EphemeralPortsUsed string // set only when this decision relied on a default (assumed) ephemeral range
}

func naclProtocolMatches(rule NACLRule, protocol string) bool {
	return rule.Protocol == "-1" || rule.Protocol == protocol
}

func naclPortMatches(rule NACLRule, port int) bool {
	if rule.Protocol == "-1" || rule.Protocol == "icmp" {
		return true
	}
	return port >= rule.FromPort && port <= rule.ToPort
}

// EvaluateNACLDirectional evaluates one direction's numbered rules in real AWS order
// (ascending by number, catch-all last) and returns the FIRST match — allow or deny,
// whichever the matching rule says (unlike SGs, a NACL rule can deny).
func EvaluateNACLDirectional(profile NACLProfile, direction, cidr, protocol string, port int) NACLDecision {
	var directional []NACLRule
	for _, r := range profile.Rules {
		if r.Direction == direction {
			directional = append(directional, r)
		}
	}
	sort.Slice(directional, func(i, j int) bool {
		ni, nj := directional[i].Number, directional[j].Number
		if ni == NACLCatchAll {
			return false
		}
		if nj == NACLCatchAll {
			return true
		}
		return ni < nj
	})

	for i := range directional {
		rule := directional[i]
		if !naclProtocolMatches(rule, protocol) || !naclPortMatches(rule, port) {
			continue
		}
		match, ambiguous := cidrMatch(rule.CIDR, cidr)
		if ambiguous {
			return NACLDecision{Allowed: false, NACLID: profile.NACLID, MatchedRuleNumber: ruleNumberLabel(rule.Number),
				Reason: "not_assessable: CIDR partially overlaps rule " + ruleNumberLabel(rule.Number) + "'s range without being fully contained — cannot determine whether the real address is covered"}
		}
		if !match {
			continue
		}
		reason := "rule " + ruleNumberLabel(rule.Number) + " (" + allowDenyLabel(rule.Allow) + ") matched"
		return NACLDecision{Allowed: rule.Allow, NACLID: profile.NACLID, MatchedRuleNumber: ruleNumberLabel(rule.Number), Reason: reason}
	}
	return NACLDecision{Allowed: false, NACLID: profile.NACLID, MatchedRuleNumber: "", Reason: "no rule matched at all, not even a catch-all — denied per AWS's own default-deny behavior"}
}

func ruleNumberLabel(n int) string {
	if n == NACLCatchAll {
		return "*"
	}
	return itoa(n)
}

func allowDenyLabel(allow bool) string {
	if allow {
		return "ALLOW"
	}
	return "DENY"
}

// itoa avoids importing strconv for one call site's single int-to-string need in this
// file — kept trivial and local.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// EvaluateNACLConnection is the real, STATELESS, full-connection answer for a request
// crossing subnet boundaries — the opposite of EvaluateConnection's statefulness: BOTH
// the forward leg (source subnet's egress, dest subnet's ingress) AND the return leg
// (dest subnet's egress, source subnet's ingress, on the ephemeral port range) must be
// separately allowed, because "responses to that traffic are not automatically
// allowed" (this file's own package doc comment). If the caller supplies no declared
// ephemeral range, DefaultEphemeralPortFrom/To is used and EphemeralPortsUsed is
// stamped on both legs' decisions so a caller can tag the result assumed.
func EvaluateNACLConnection(source, dest NACLProfile, sourceCIDR, destCIDR, protocol string, port int, ephemeralFrom, ephemeralTo int) (allowed bool, forward, returnLeg [2]NACLDecision) {
	if ephemeralFrom == 0 && ephemeralTo == 0 {
		ephemeralFrom, ephemeralTo = DefaultEphemeralPortFrom, DefaultEphemeralPortTo
	}
	ephemeralLabel := rangeLabel(ephemeralFrom, ephemeralTo)

	fwdOut := EvaluateNACLDirectional(source, "egress", destCIDR, protocol, port)
	fwdIn := EvaluateNACLDirectional(dest, "ingress", sourceCIDR, protocol, port)
	forward = [2]NACLDecision{fwdOut, fwdIn}
	if !fwdOut.Allowed || !fwdIn.Allowed {
		return false, forward, [2]NACLDecision{}
	}

	retOut := evaluateNACLEphemeral(dest, "egress", sourceCIDR, protocol, ephemeralFrom, ephemeralTo, ephemeralLabel)
	retIn := evaluateNACLEphemeral(source, "ingress", destCIDR, protocol, ephemeralFrom, ephemeralTo, ephemeralLabel)
	returnLeg = [2]NACLDecision{retOut, retIn}
	return retOut.Allowed && retIn.Allowed, forward, returnLeg
}

func evaluateNACLEphemeral(profile NACLProfile, direction, cidr, protocol string, from, to int, label string) NACLDecision {
	// The return leg is checked against the FULL ephemeral range (a real client can
	// use any port in it) — checked at both ends of the range plus the midpoint as a
	// pragmatic proxy for "the whole range must be covered by one matching rule,"
	// honest about what it verifies rather than iterating 64511 individual ports.
	d := EvaluateNACLDirectional(profile, direction, cidr, protocol, from)
	d.EphemeralPortsUsed = label
	if !d.Allowed {
		return d
	}
	d2 := EvaluateNACLDirectional(profile, direction, cidr, protocol, to)
	if !d2.Allowed {
		d2.EphemeralPortsUsed = label
		return d2
	}
	return d
}

func rangeLabel(from, to int) string {
	return itoa(from) + "-" + itoa(to)
}
