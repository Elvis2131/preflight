// Package networking holds PC-112's real conformance tests — Security Group
// evaluation. See tests/aws-conformance/harness for the format these tests use.
package networking

import (
	"testing"

	"preflight/core"
	"preflight/tests/aws-conformance/harness"
)

func TestSG_AllowOnly_001(t *testing.T) {
	spec := harness.Verify(t, harness.Spec{
		ID:            "SG-STATEFUL-001",
		Rule:          "Security groups support allow rules only — there is no deny rule. Anything not explicitly allowed is denied.",
		Source:        "https://docs.aws.amazon.com/vpc/latest/userguide/security-group-rules.html",
		Scenario:      "A security group has one ingress rule allowing tcp/443 from 0.0.0.0/0.",
		Configuration: "ingress = [{protocol: tcp, from_port: 443, to_port: 443, cidr_blocks: [0.0.0.0/0]}]",
		Request:       "Evaluate an inbound request on tcp/22.",
		Expected:      "Denied — no allow rule matches, and there is no deny rule to construct in the first place.",
	})

	profile := core.SGProfile{SGIDs: []string{"sg-web"}, Rules: []core.SGRule{
		{Direction: "ingress", Protocol: "tcp", FromPort: 443, ToPort: 443, CIDRs: []string{"0.0.0.0/0"}},
	}}
	initiator := core.SGProfile{SGIDs: []string{"sg-client"}, Rules: []core.SGRule{
		{Direction: "egress", Protocol: "-1", FromPort: 0, ToPort: 0},
	}}
	allowed, _, resp := core.EvaluateConnection(initiator, profile, "203.0.113.1/32", "", "tcp", 22)
	if allowed {
		t.Fatalf("%s (%s): got allowed, want denied — see %s (%+v)", spec.ID, spec.Rule, spec.Source, resp)
	}
}

func TestSG_Stateful_002(t *testing.T) {
	spec := harness.Verify(t, harness.Spec{
		ID:            "SG-STATEFUL-002",
		Rule:          "Security groups are stateful: responses to inbound traffic are allowed to flow out regardless of outbound rules, and vice versa.",
		Source:        "https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/security-group-connection-tracking.html",
		Scenario:      "A client with an egress-allow rule connects to a web server with an ingress-allow rule; NEITHER side has a rule for the return leg.",
		Configuration: "client egress: tcp/443 to 0.0.0.0/0; web ingress: tcp/443 from 0.0.0.0/0; no other rules on either side",
		Request:       "Evaluate the full connection (both legs).",
		Expected:      "Allowed — the return leg is never separately checked.",
	})

	client := core.SGProfile{SGIDs: []string{"sg-client"}, Rules: []core.SGRule{
		{Direction: "egress", Protocol: "tcp", FromPort: 443, ToPort: 443, CIDRs: []string{"0.0.0.0/0"}},
	}}
	web := core.SGProfile{SGIDs: []string{"sg-web"}, Rules: []core.SGRule{
		{Direction: "ingress", Protocol: "tcp", FromPort: 443, ToPort: 443, CIDRs: []string{"0.0.0.0/0"}},
	}}
	allowed, _, _ := core.EvaluateConnection(client, web, "203.0.113.1/32", "10.0.0.5/32", "tcp", 443)
	if !allowed {
		t.Fatalf("%s (%s): got denied, want allowed — see %s", spec.ID, spec.Rule, spec.Source)
	}
}

func TestSG_ReferenceSource_003(t *testing.T) {
	spec := harness.Verify(t, harness.Spec{
		ID:            "SG-REFERENCE-003",
		Rule:          "A security group rule can reference another security group as its source — membership-based, not IP-based.",
		Source:        "https://docs.aws.amazon.com/vpc/latest/userguide/security-group-rules.html",
		Scenario:      "AWS's own documented three-tier example: ALB -> web -> DB, each tier's SG referencing the previous tier's SG as its source.",
		Configuration: "db ingress: tcp/5432 from sg-web (a security group reference, no CIDR at all)",
		Request:       "Evaluate a connection from a resource whose own attached SG is sg-web.",
		Expected:      "Allowed — membership in sg-web is what the rule checks, not any particular IP address.",
	})

	web := core.SGProfile{SGIDs: []string{"sg-web"}, Rules: []core.SGRule{
		{Direction: "egress", Protocol: "-1", CIDRs: []string{"0.0.0.0/0"}},
	}}
	db := core.SGProfile{SGIDs: []string{"sg-db"}, Rules: []core.SGRule{
		{Direction: "ingress", Protocol: "tcp", FromPort: 5432, ToPort: 5432, SourceSG: "sg-web"},
	}}
	allowed, _, resp := core.EvaluateConnection(web, db, "", "10.0.0.5/32", "tcp", 5432)
	if !allowed {
		t.Fatalf("%s (%s): got denied, want allowed — see %s (%+v)", spec.ID, spec.Rule, spec.Source, resp)
	}
}

func TestSG_MultiSGUnion_004(t *testing.T) {
	spec := harness.Verify(t, harness.Spec{
		ID:            "SG-UNION-004",
		Rule:          "When a resource has multiple security groups, the rules from each are aggregated into a single set used to determine access.",
		Source:        "https://docs.aws.amazon.com/vpc/latest/userguide/security-group-rules.html",
		Scenario:      "A destination has two security groups attached; one allows tcp/80, the other tcp/443 — neither alone covers both.",
		Configuration: "sg-a ingress: tcp/80 from 0.0.0.0/0; sg-b ingress: tcp/443 from 0.0.0.0/0",
		Request:       "Evaluate inbound requests on both tcp/80 and tcp/443.",
		Expected:      "Both allowed — the union of the two groups' rules, not either group evaluated alone.",
	})

	dest := core.SGProfile{SGIDs: []string{"sg-a", "sg-b"}, Rules: []core.SGRule{
		{Direction: "ingress", Protocol: "tcp", FromPort: 80, ToPort: 80, CIDRs: []string{"0.0.0.0/0"}},
		{Direction: "ingress", Protocol: "tcp", FromPort: 443, ToPort: 443, CIDRs: []string{"0.0.0.0/0"}},
	}}
	client := core.SGProfile{Rules: []core.SGRule{{Direction: "egress", Protocol: "-1", CIDRs: []string{"0.0.0.0/0"}}}}
	for _, port := range []int{80, 443} {
		allowed, _, resp := core.EvaluateConnection(client, dest, "203.0.113.1/32", "10.0.0.5/32", "tcp", port)
		if !allowed {
			t.Fatalf("%s (%s): port %d got denied, want allowed — see %s (%+v)", spec.ID, spec.Rule, port, spec.Source, resp)
		}
	}
}

func TestSG_ProtocolPort_005(t *testing.T) {
	spec := harness.Verify(t, harness.Spec{
		ID:            "SG-PORT-005",
		Rule:          "A rule's protocol and port range must both match the request's protocol and port for the rule to apply.",
		Source:        "https://docs.aws.amazon.com/vpc/latest/userguide/security-group-rules.html",
		Scenario:      "A rule allows tcp/7000-8000 only.",
		Configuration: "ingress: tcp, port range 7000-8000, from 0.0.0.0/0",
		Request:       "Evaluate requests on tcp/7500 (inside range) and tcp/9000 (outside range).",
		Expected:      "tcp/7500 allowed, tcp/9000 denied.",
	})

	dest := core.SGProfile{Rules: []core.SGRule{
		{Direction: "ingress", Protocol: "tcp", FromPort: 7000, ToPort: 8000, CIDRs: []string{"0.0.0.0/0"}},
	}}
	client := core.SGProfile{Rules: []core.SGRule{{Direction: "egress", Protocol: "-1", CIDRs: []string{"0.0.0.0/0"}}}}
	if allowed, _, _ := core.EvaluateConnection(client, dest, "1.2.3.4/32", "10.0.0.5/32", "tcp", 7500); !allowed {
		t.Fatalf("%s (%s): port 7500 got denied, want allowed — see %s", spec.ID, spec.Rule, spec.Source)
	}
	if allowed, _, _ := core.EvaluateConnection(client, dest, "1.2.3.4/32", "10.0.0.5/32", "tcp", 9000); allowed {
		t.Fatalf("%s (%s): port 9000 got allowed, want denied — see %s", spec.ID, spec.Rule, spec.Source)
	}
}
