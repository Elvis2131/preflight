package analyse

import "testing"

// TestNACL_FirstMatchByRuleNumber is PC-113's own acceptance criterion, cited against
// AWS's own "we evaluate the rules in order, starting with the lowest numbered rule
// ... If the traffic matches a rule, the rule is applied and we do not evaluate any
// additional rules." A lower-numbered DENY must win over a higher-numbered ALLOW that
// would otherwise also match.
func TestNACL_FirstMatchByRuleNumber(t *testing.T) {
	profile := NACLProfile{NACLID: "acl-1", Rules: []NACLRule{
		{Number: 50, Direction: "ingress", Protocol: "tcp", FromPort: 22, ToPort: 22, CIDR: "203.0.113.5/32", Allow: false},
		{Number: 100, Direction: "ingress", Protocol: "tcp", FromPort: 22, ToPort: 22, CIDR: "0.0.0.0/0", Allow: true},
		{Number: NACLCatchAll, Direction: "ingress", Protocol: "-1", CIDR: "0.0.0.0/0", Allow: false},
	}}
	d := EvaluateNACLDirectional(profile, "ingress", "203.0.113.5/32", "tcp", 22)
	if d.Allowed || d.MatchedRuleNumber != "50" {
		t.Fatalf("got %+v, want denied by rule 50 (lower-numbered, evaluated first)", d)
	}
}

func TestNACL_ExplicitDenyRule(t *testing.T) {
	profile := NACLProfile{NACLID: "acl-1", Rules: []NACLRule{
		{Number: 100, Direction: "ingress", Protocol: "-1", CIDR: "198.51.100.0/24", Allow: false},
		{Number: NACLCatchAll, Direction: "ingress", Protocol: "-1", CIDR: "0.0.0.0/0", Allow: false},
	}}
	d := EvaluateNACLDirectional(profile, "ingress", "198.51.100.5/32", "tcp", 80)
	if d.Allowed || d.MatchedRuleNumber != "100" {
		t.Fatalf("got %+v, want denied by the explicit numbered deny rule (not the catch-all)", d)
	}
}

// TestNACL_CatchAllDeny is PC-113's own acceptance criterion, cited against AWS's own
// "Each network ACL also includes rules where the rule number is an asterisk (*).
// These rules ensure that if a packet doesn't match any of the other numbered rules,
// it's denied."
func TestNACL_CatchAllDeny(t *testing.T) {
	profile := NACLProfile{NACLID: "acl-1", Rules: []NACLRule{
		{Number: 100, Direction: "ingress", Protocol: "tcp", FromPort: 22, ToPort: 22, CIDR: "203.0.113.5/32", Allow: true},
		{Number: NACLCatchAll, Direction: "ingress", Protocol: "-1", CIDR: "0.0.0.0/0", Allow: false},
	}}
	d := EvaluateNACLDirectional(profile, "ingress", "198.51.100.1/32", "tcp", 443)
	if d.Allowed || d.MatchedRuleNumber != "*" {
		t.Fatalf("got %+v, want denied by the catch-all rule", d)
	}
}

func TestNACL_Allow(t *testing.T) {
	profile := NACLProfile{NACLID: "acl-1", Rules: []NACLRule{
		{Number: 100, Direction: "ingress", Protocol: "tcp", FromPort: 22, ToPort: 22, CIDR: "203.0.113.5/32", Allow: true},
		{Number: NACLCatchAll, Direction: "ingress", Protocol: "-1", CIDR: "0.0.0.0/0", Allow: false},
	}}
	d := EvaluateNACLDirectional(profile, "ingress", "203.0.113.5/32", "tcp", 22)
	if !d.Allowed || d.MatchedRuleNumber != "100" {
		t.Fatalf("got %+v, want allowed by rule 100", d)
	}
}

// TestNACL_Stateless_MissingReturnRuleFailsTheConnection is PC-113's own acceptance
// criterion, verbatim: "Stateless return-traffic case: inbound allowed, outbound
// return missing -> request fails at the NACL, test proves it." Cited against AWS's
// own contrast with SGs: "NACLs are stateless ... responses to that traffic are not
// automatically allowed."
func TestNACL_Stateless_MissingReturnRuleFailsTheConnection(t *testing.T) {
	source := NACLProfile{NACLID: "acl-client", Rules: []NACLRule{
		{Number: 100, Direction: "egress", Protocol: "tcp", FromPort: 443, ToPort: 443, CIDR: "0.0.0.0/0", Allow: true},
		{Number: NACLCatchAll, Direction: "ingress", Protocol: "-1", CIDR: "0.0.0.0/0", Allow: false},
		{Number: NACLCatchAll, Direction: "egress", Protocol: "-1", CIDR: "0.0.0.0/0", Allow: false},
		// Deliberately NO ingress rule allowing the ephemeral-port return traffic.
	}}
	dest := NACLProfile{NACLID: "acl-web", Rules: []NACLRule{
		{Number: 100, Direction: "ingress", Protocol: "tcp", FromPort: 443, ToPort: 443, CIDR: "0.0.0.0/0", Allow: true},
		{Number: 100, Direction: "egress", Protocol: "tcp", FromPort: 1024, ToPort: 65535, CIDR: "0.0.0.0/0", Allow: true},
		{Number: NACLCatchAll, Direction: "ingress", Protocol: "-1", CIDR: "0.0.0.0/0", Allow: false},
		{Number: NACLCatchAll, Direction: "egress", Protocol: "-1", CIDR: "0.0.0.0/0", Allow: false},
	}}

	allowed, forward, returnLeg := EvaluateNACLConnection(source, dest, "203.0.113.1/32", "10.0.0.5/32", "tcp", 443, 0, 0)
	if allowed {
		t.Fatalf("got allowed=true, want false — the client's own NACL has no rule permitting the ephemeral-port return traffic; forward=%+v return=%+v", forward, returnLeg)
	}
	if !forward[0].Allowed || !forward[1].Allowed {
		t.Fatalf("the forward leg itself should have been allowed (that's not what's under test here): %+v", forward)
	}
}

func TestNACL_Stateless_FullRoundTripAllowedWhenBothLegsPermitted(t *testing.T) {
	source := NACLProfile{NACLID: "acl-client", Rules: []NACLRule{
		{Number: 100, Direction: "egress", Protocol: "tcp", FromPort: 443, ToPort: 443, CIDR: "0.0.0.0/0", Allow: true},
		{Number: 100, Direction: "ingress", Protocol: "tcp", FromPort: 1024, ToPort: 65535, CIDR: "0.0.0.0/0", Allow: true},
		{Number: NACLCatchAll, Direction: "ingress", Protocol: "-1", CIDR: "0.0.0.0/0", Allow: false},
		{Number: NACLCatchAll, Direction: "egress", Protocol: "-1", CIDR: "0.0.0.0/0", Allow: false},
	}}
	dest := NACLProfile{NACLID: "acl-web", Rules: []NACLRule{
		{Number: 100, Direction: "ingress", Protocol: "tcp", FromPort: 443, ToPort: 443, CIDR: "0.0.0.0/0", Allow: true},
		{Number: 100, Direction: "egress", Protocol: "tcp", FromPort: 1024, ToPort: 65535, CIDR: "0.0.0.0/0", Allow: true},
		{Number: NACLCatchAll, Direction: "ingress", Protocol: "-1", CIDR: "0.0.0.0/0", Allow: false},
		{Number: NACLCatchAll, Direction: "egress", Protocol: "-1", CIDR: "0.0.0.0/0", Allow: false},
	}}
	allowed, _, returnLeg := EvaluateNACLConnection(source, dest, "203.0.113.1/32", "10.0.0.5/32", "tcp", 443, 0, 0)
	if !allowed {
		t.Fatalf("got denied, want allowed — both legs are explicitly permitted: %+v", returnLeg)
	}
	if returnLeg[0].EphemeralPortsUsed != "1024-65535" {
		t.Errorf("EphemeralPortsUsed = %q, want the default range labeled, since none was declared", returnLeg[0].EphemeralPortsUsed)
	}
}
