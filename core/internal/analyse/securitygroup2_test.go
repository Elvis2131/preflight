package analyse

import "testing"

// TestSG_AllowOnly_ImplicitDeny is PC-112's own acceptance criterion, verbatim, cited
// against docs.aws.amazon.com/vpc/latest/userguide/security-group-rules.html: "You can
// specify allow rules, but not deny rules." A profile with rules that don't match the
// request must deny — there is no "explicit deny rule" to construct in the first
// place, since AWS's own model has none.
func TestSG_AllowOnly_ImplicitDeny(t *testing.T) {
	profile := SGProfile{SGIDs: []string{"sg-web"}, Rules: []SGRule{
		{Direction: "ingress", Protocol: "tcp", FromPort: 443, ToPort: 443, CIDRs: []string{"0.0.0.0/0"}},
	}}
	d := EvaluateDirectional(profile, "ingress", "203.0.113.1/32", nil, "tcp", 22)
	if d.Allowed {
		t.Fatalf("port 22 has no matching rule; want implicit deny, got %+v", d)
	}
	if d.Reason == "" {
		t.Error("a denied decision must always state a reason")
	}
}

func TestSG_CIDRSourced_Allowed(t *testing.T) {
	profile := SGProfile{SGIDs: []string{"sg-web"}, Rules: []SGRule{
		{Direction: "ingress", Protocol: "tcp", FromPort: 443, ToPort: 443, CIDRs: []string{"0.0.0.0/0"}},
	}}
	d := EvaluateDirectional(profile, "ingress", "203.0.113.1/32", nil, "tcp", 443)
	if !d.Allowed || d.MatchedSG != "sg-web" {
		t.Fatalf("got %+v, want allowed by sg-web", d)
	}
}

// TestSG_ReferenceSource_MembershipBased is PC-112's own acceptance criterion: "Rule
// sources/destinations: CIDR ranges and references to other security groups
// (membership-based, not IP-based)." Cited against security-group-rules.html's own
// "Security group referencing" section and its worked example (ALB -> web -> DB).
func TestSG_ReferenceSource_MembershipBased(t *testing.T) {
	dbProfile := SGProfile{SGIDs: []string{"sg-db"}, Rules: []SGRule{
		{Direction: "ingress", Protocol: "tcp", FromPort: 5432, ToPort: 5432, SourceSG: "sg-web"},
	}}
	// The actual source's own attached SGs include sg-web — membership match, no CIDR involved at all.
	d := EvaluateDirectional(dbProfile, "ingress", "", []string{"sg-web"}, "tcp", 5432)
	if !d.Allowed {
		t.Fatalf("got %+v, want allowed — source is a member of sg-web, the referenced group", d)
	}

	// A different source, NOT a member of sg-web, must be denied even with no CIDR at all involved.
	d2 := EvaluateDirectional(dbProfile, "ingress", "", []string{"sg-other"}, "tcp", 5432)
	if d2.Allowed {
		t.Fatalf("got %+v, want denied — source is not a member of sg-web", d2)
	}
}

// TestSG_MultiSGUnion is PC-112's own acceptance criterion: "Evaluated at the ENI of
// the resource; a resource can have multiple SGs, evaluated as the union of their
// rules." Cited against security-group-rules.html: "the rules from each security
// group are aggregated to form a single set of rules."
func TestSG_MultiSGUnion(t *testing.T) {
	profile := SGProfile{SGIDs: []string{"sg-a", "sg-b"}, Rules: []SGRule{
		{Direction: "ingress", Protocol: "tcp", FromPort: 80, ToPort: 80, CIDRs: []string{"10.0.0.0/8"}},   // from sg-a
		{Direction: "ingress", Protocol: "tcp", FromPort: 443, ToPort: 443, CIDRs: []string{"10.0.0.0/8"}}, // from sg-b
	}}
	for _, port := range []int{80, 443} {
		d := EvaluateDirectional(profile, "ingress", "10.1.2.3/32", nil, "tcp", port)
		if !d.Allowed {
			t.Errorf("port %d: got %+v, want allowed — the union of sg-a and sg-b together permit it, neither alone would", port, d)
		}
	}
}

func TestSG_ProtocolAndPortRangeMatching(t *testing.T) {
	profile := SGProfile{SGIDs: []string{"sg"}, Rules: []SGRule{
		{Direction: "ingress", Protocol: "tcp", FromPort: 7000, ToPort: 8000, CIDRs: []string{"0.0.0.0/0"}},
	}}
	if d := EvaluateDirectional(profile, "ingress", "1.2.3.4/32", nil, "tcp", 7500); !d.Allowed {
		t.Errorf("port 7500 is inside 7000-8000, want allowed, got %+v", d)
	}
	if d := EvaluateDirectional(profile, "ingress", "1.2.3.4/32", nil, "tcp", 9000); d.Allowed {
		t.Errorf("port 9000 is outside 7000-8000, want denied, got %+v", d)
	}
	if d := EvaluateDirectional(profile, "ingress", "1.2.3.4/32", nil, "udp", 7500); d.Allowed {
		t.Errorf("protocol udp != tcp, want denied, got %+v", d)
	}
}

// TestSG_Statefulness_ReturnTrafficNeedsNoExplicitRule is PC-112's own acceptance
// criterion, verbatim: "Return traffic allowed without an explicit return rule (the
// statefulness test the reference project's own guidance calls out)." Cited against
// docs.aws.amazon.com/AWSEC2/latest/UserGuide/security-group-connection-tracking.html:
// "responses to inbound traffic are allowed to flow out of the instance regardless of
// outbound security group rules, and vice versa." The web tier here has NO egress
// rule permitting traffic back to the client at all — EvaluateConnection must still
// report the connection allowed, because AWS's own statefulness means the return leg
// is never separately checked.
func TestSG_Statefulness_ReturnTrafficNeedsNoExplicitRule(t *testing.T) {
	client := SGProfile{SGIDs: []string{"sg-client"}, Rules: []SGRule{
		{Direction: "egress", Protocol: "tcp", FromPort: 443, ToPort: 443, CIDRs: []string{"0.0.0.0/0"}},
		// Deliberately NO ingress rule at all — if this engine incorrectly re-checked
		// the return leg against the client's own ingress rules, this test would fail.
	}}
	web := SGProfile{SGIDs: []string{"sg-web"}, Rules: []SGRule{
		{Direction: "ingress", Protocol: "tcp", FromPort: 443, ToPort: 443, CIDRs: []string{"0.0.0.0/0"}},
		// Deliberately NO egress rule at all — same statefulness claim, the other side.
	}}

	allowed, initDecision, respDecision := EvaluateConnection(client, web, "203.0.113.1/32", "10.0.0.5/32", "tcp", 443)
	if !allowed {
		t.Fatalf("got allowed=false, want true — statefulness means neither side needs a return-path rule; initiator=%+v responder=%+v", initDecision, respDecision)
	}
}

func TestSG_Statefulness_InitiatorEgressStillGatesTheForwardLeg(t *testing.T) {
	client := SGProfile{SGIDs: []string{"sg-client"}} // no egress rules at all: cannot initiate anything
	web := SGProfile{SGIDs: []string{"sg-web"}, Rules: []SGRule{
		{Direction: "ingress", Protocol: "tcp", FromPort: 443, ToPort: 443, CIDRs: []string{"0.0.0.0/0"}},
	}}
	allowed, initDecision, _ := EvaluateConnection(client, web, "203.0.113.1/32", "10.0.0.5/32", "tcp", 443)
	if allowed {
		t.Fatal("want denied — the initiator's own egress rules block this, statefulness doesn't bypass the forward leg")
	}
	if initDecision.Allowed {
		t.Error("initiator's own decision should itself report denied")
	}
}

// TestSG_AmbiguousCIDR_NotAssessable is PC-112's own reconciliation note: "Where [a
// CIDR match] is ambiguous (source subnet only partially inside the rule's range),
// return not_assessable with that reason rather than guessing allow or deny."
func TestSG_AmbiguousCIDR_NotAssessable(t *testing.T) {
	profile := SGProfile{SGIDs: []string{"sg"}, Rules: []SGRule{
		{Direction: "ingress", Protocol: "tcp", FromPort: 443, ToPort: 443, CIDRs: []string{"10.0.0.0/24"}},
	}}
	// sourceCIDR is a /16 that overlaps but is NOT fully contained by the rule's /24.
	d := EvaluateDirectional(profile, "ingress", "10.0.0.0/16", nil, "tcp", 443)
	if d.Allowed {
		t.Fatalf("got %+v, want NOT allowed — a partial overlap must never be silently treated as a match", d)
	}
	if d.Reason == "" {
		t.Error("expected a stated not_assessable-style reason naming the ambiguity")
	}
}
