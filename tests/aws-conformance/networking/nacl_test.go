package networking

// PC-113's real conformance tests — Network ACL evaluation.

import (
	"testing"

	"preflight/core"
	"preflight/tests/aws-conformance/harness"
)

func TestNACL_Order_001(t *testing.T) {
	spec := harness.Verify(t, harness.Spec{
		ID:            "NACL-ORDER-001",
		Rule:          "NACL rules are evaluated in ascending order by rule number; the first match decides, regardless of any higher-numbered rule that might contradict it.",
		Source:        "https://docs.aws.amazon.com/vpc/latest/userguide/vpc-network-acls.html",
		Scenario:      "A lower-numbered DENY rule and a higher-numbered ALLOW rule both match the same traffic.",
		Configuration: "rule 50: deny tcp/22 from 203.0.113.5/32; rule 100: allow tcp/22 from 0.0.0.0/0",
		Request:       "Evaluate inbound tcp/22 from 203.0.113.5.",
		Expected:      "Denied by rule 50 — the lower-numbered rule wins.",
	})

	profile := core.NACLProfile{NACLID: "acl-1", Rules: []core.NACLRule{
		{Number: 50, Direction: "ingress", Protocol: "tcp", FromPort: 22, ToPort: 22, CIDR: "203.0.113.5/32", Allow: false},
		{Number: 100, Direction: "ingress", Protocol: "tcp", FromPort: 22, ToPort: 22, CIDR: "0.0.0.0/0", Allow: true},
	}}
	d := core.EvaluateNACLDirectional(profile, "ingress", "203.0.113.5/32", "tcp", 22)
	if d.Allowed || d.MatchedRuleNumber != "50" {
		t.Fatalf("%s (%s): got %+v, want denied by rule 50 — see %s", spec.ID, spec.Rule, d, spec.Source)
	}
}

func TestNACL_CatchAll_002(t *testing.T) {
	spec := harness.Verify(t, harness.Spec{
		ID:            "NACL-ORDER-002",
		Rule:          "Every NACL includes a final catch-all rule (rule number *) that denies any traffic not matched by an earlier numbered rule.",
		Source:        "https://docs.aws.amazon.com/vpc/latest/userguide/default-network-acl.html",
		Scenario:      "A NACL has one specific allow rule and the standard catch-all.",
		Configuration: "rule 100: allow tcp/22 from 203.0.113.5/32; rule *: deny all",
		Request:       "Evaluate inbound tcp/443 from an unrelated address.",
		Expected:      "Denied by the catch-all rule (*), since nothing else matched.",
	})

	profile := core.NACLProfile{NACLID: "acl-1", Rules: []core.NACLRule{
		{Number: 100, Direction: "ingress", Protocol: "tcp", FromPort: 22, ToPort: 22, CIDR: "203.0.113.5/32", Allow: true},
		{Number: core.NACLCatchAll, Direction: "ingress", Protocol: "-1", CIDR: "0.0.0.0/0", Allow: false},
	}}
	d := core.EvaluateNACLDirectional(profile, "ingress", "198.51.100.1/32", "tcp", 443)
	if d.Allowed || d.MatchedRuleNumber != "*" {
		t.Fatalf("%s (%s): got %+v, want denied by the catch-all — see %s", spec.ID, spec.Rule, d, spec.Source)
	}
}

func TestNACL_Stateless_003(t *testing.T) {
	spec := harness.Verify(t, harness.Spec{
		ID:            "NACL-STATELESS-003",
		Rule:          "NACLs are stateless: a rule allowing inbound traffic does not automatically allow the response — the return path needs its own explicit rule.",
		Source:        "https://docs.aws.amazon.com/vpc/latest/userguide/vpc-network-acls.html",
		Scenario:      "A web server's NACL allows inbound tcp/443 but has no outbound rule for the ephemeral-port response.",
		Configuration: "web ingress: allow tcp/443 from 0.0.0.0/0; web egress: only the catch-all deny",
		Request:       "Evaluate the full connection (both legs).",
		Expected:      "Denied overall — the forward leg is allowed, but the connection still fails because the return leg has no matching allow rule.",
	})

	client := core.NACLProfile{NACLID: "acl-client", Rules: []core.NACLRule{
		{Number: 100, Direction: "egress", Protocol: "tcp", FromPort: 443, ToPort: 443, CIDR: "0.0.0.0/0", Allow: true},
		{Number: core.NACLCatchAll, Direction: "ingress", Protocol: "-1", CIDR: "0.0.0.0/0", Allow: false},
		{Number: core.NACLCatchAll, Direction: "egress", Protocol: "-1", CIDR: "0.0.0.0/0", Allow: false},
	}}
	web := core.NACLProfile{NACLID: "acl-web", Rules: []core.NACLRule{
		{Number: 100, Direction: "ingress", Protocol: "tcp", FromPort: 443, ToPort: 443, CIDR: "0.0.0.0/0", Allow: true},
		{Number: core.NACLCatchAll, Direction: "ingress", Protocol: "-1", CIDR: "0.0.0.0/0", Allow: false},
		{Number: core.NACLCatchAll, Direction: "egress", Protocol: "-1", CIDR: "0.0.0.0/0", Allow: false},
	}}
	evaluated, allowed, _, _ := core.EvaluateNACLPath("subnet.client", "subnet.web", client, web, "203.0.113.1/32", "10.0.0.5/32", "tcp", 443, 0, 0)
	if !evaluated {
		t.Fatalf("%s (%s): cross-subnet traffic must evaluate the NACL", spec.ID, spec.Rule)
	}
	if allowed {
		t.Fatalf("%s (%s): got allowed, want denied — see %s", spec.ID, spec.Rule, spec.Source)
	}
}

func TestNACL_SameSubnet_004(t *testing.T) {
	spec := harness.Verify(t, harness.Spec{
		ID:            "NACL-SUBNET-004",
		Rule:          "Network ACL rules are evaluated when traffic enters and leaves the subnet, not as it is routed within a subnet.",
		Source:        "https://docs.aws.amazon.com/vpc/latest/userguide/vpc-network-acls.html",
		Scenario:      "Two resources in the SAME subnet, whose NACL would deny everything if it were ever actually evaluated.",
		Configuration: "the subnet's own NACL: catch-all deny on both ingress and egress",
		Request:       "Evaluate traffic between two resources in the same subnet.",
		Expected:      "The NACL is never evaluated at all — traffic within one subnet doesn't cross it.",
	})

	denyAll := core.NACLProfile{NACLID: "acl-x", Rules: []core.NACLRule{
		{Number: core.NACLCatchAll, Direction: "ingress", Protocol: "-1", CIDR: "0.0.0.0/0", Allow: false},
		{Number: core.NACLCatchAll, Direction: "egress", Protocol: "-1", CIDR: "0.0.0.0/0", Allow: false},
	}}
	evaluated, allowed, _, _ := core.EvaluateNACLPath("subnet.a", "subnet.a", denyAll, denyAll, "10.0.0.1/32", "10.0.0.2/32", "tcp", 443, 0, 0)
	if evaluated {
		t.Fatalf("%s (%s): the NACL must not be evaluated for same-subnet traffic — see %s", spec.ID, spec.Rule, spec.Source)
	}
	if !allowed {
		t.Fatalf("%s (%s): same-subnet traffic must not be blocked by a NACL that never applied", spec.ID, spec.Rule)
	}
}
