package placement

import (
	"strings"
	"testing"

	"preflight/core"
	"preflight/tests/aws-conformance/harness"
)

func node(id, serviceID string) core.CanvasNode {
	return core.CanvasNode{ID: id, Type: "network_boundary", Label: id, Capability: map[string]string{}, ServiceID: serviceID}
}

func subnet(id, az string) core.CanvasNode {
	n := node(id, "aws_subnet")
	n.AvailabilityZone = az
	return n
}

func in(id, from, to string) core.CanvasEdge {
	return core.CanvasEdge{ID: id, Type: "contained_in", From: from, To: to}
}

func rules(vs []core.PlacementViolation) string {
	var out []string
	for _, v := range vs {
		out = append(out, v.Rule+"@"+v.ResourceID)
	}
	return strings.Join(out, ",")
}

func TestPlacement_SubnetBelongsToOneVPC_001(t *testing.T) {
	spec := harness.Verify(t, harness.Spec{
		ID:            "PLACE-SUBNET-ONE-VPC-001",
		Rule:          "A subnet is a range of IP addresses in a single VPC.",
		Source:        "https://docs.aws.amazon.com/vpc/latest/userguide/configure-subnets.html",
		Scenario:      "A subnet is drawn inside no VPC, inside one VPC, and inside two VPCs.",
		Configuration: "subnets s-none (no contained_in), s-one (in vpc1), s-two (in vpc1 and vpc2)",
		Request:       "ValidateCanvasPlacement over the document.",
		Expected:      "Violations for s-none and s-two only.",
	})
	doc := core.CanvasDocument{
		Nodes: []core.CanvasNode{node("vpc1", "aws_vpc"), node("vpc2", "aws_vpc"), subnet("s-none", "eu-west-1a"), subnet("s-one", "eu-west-1a"), subnet("s-two", "eu-west-1a")},
		Edges: []core.CanvasEdge{in("e1", "s-one", "vpc1"), in("e2", "s-two", "vpc1"), in("e3", "s-two", "vpc2")},
	}
	if got, want := rules(core.ValidateCanvasPlacement(doc)), "PLACE-SUBNET-ONE-VPC@s-none,PLACE-SUBNET-ONE-VPC@s-two"; got != want {
		t.Fatalf("%s (%s): got %q, want %q — see %s", spec.ID, spec.Rule, got, want, spec.Source)
	}
}

func TestPlacement_NATGatewayInExactlyOneSubnet_001(t *testing.T) {
	spec := harness.Verify(t, harness.Spec{
		ID:            "PLACE-NAT-ONE-SUBNET-001",
		Rule:          "A NAT gateway is created in one specific subnet (\"Select the subnet in which to create the NAT gateway\"). Regional NAT gateways, which expand across AZs automatically, are not modelled.",
		Source:        "https://docs.aws.amazon.com/vpc/latest/userguide/nat-gateway-working-with.html",
		Scenario:      "A NAT gateway drawn inside no subnet, one subnet, and two subnets.",
		Configuration: "nat-none, nat-one (in s1), nat-two (in s1 and s2)",
		Request:       "ValidateCanvasPlacement over the document.",
		Expected:      "Violations for nat-none and nat-two only.",
	})
	doc := core.CanvasDocument{
		Nodes: []core.CanvasNode{node("vpc", "aws_vpc"), subnet("s1", "eu-west-1a"), subnet("s2", "eu-west-1b"),
			node("nat-none", "aws_nat_gateway"), node("nat-one", "aws_nat_gateway"), node("nat-two", "aws_nat_gateway")},
		Edges: []core.CanvasEdge{in("e1", "s1", "vpc"), in("e2", "s2", "vpc"), in("e3", "nat-one", "s1"), in("e4", "nat-two", "s1"), in("e5", "nat-two", "s2")},
	}
	if got, want := rules(core.ValidateCanvasPlacement(doc)), "PLACE-NAT-ONE-SUBNET@nat-none,PLACE-NAT-ONE-SUBNET@nat-two"; got != want {
		t.Fatalf("%s (%s): got %q, want %q — see %s", spec.ID, spec.Rule, got, want, spec.Source)
	}
}

func alb(id string) core.CanvasNode {
	n := node(id, "aws_lb")
	n.Type = "load_balancer"
	n.Sizing = map[string]string{"load_balancer_type": "application"}
	return n
}

func TestPlacement_ALBNeedsTwoAvailabilityZones_001(t *testing.T) {
	spec := harness.Verify(t, harness.Spec{
		ID:            "PLACE-ALB-TWO-AZS-001",
		Rule:          "An Application Load Balancer must be given at least two Availability Zone subnets, each from a different Availability Zone (\"You must select at least two Availability Zone subnets... Each subnet must be from a different Availability Zone\"). Local Zone and Outpost subnets are not modelled.",
		Source:        "https://docs.aws.amazon.com/elasticloadbalancing/latest/application/application-load-balancers.html",
		Scenario:      "ALBs in one subnet, in two subnets of the SAME zone, in two subnets of different zones, and in two subnets where one zone is not yet declared.",
		Configuration: "alb-1sub (s1a), alb-samezone (s1a,s1a2), alb-ok (s1a,s1b), alb-undeclared (s1a, s-noaz)",
		Request:       "ValidateCanvasPlacement over the document.",
		Expected:      "Violations for alb-1sub and alb-samezone only; alb-undeclared is not assessable yet (incomplete is never a violation, I4).",
	})
	doc := core.CanvasDocument{
		Nodes: []core.CanvasNode{node("vpc", "aws_vpc"), subnet("s1a", "eu-west-1a"), subnet("s1a2", "eu-west-1a"), subnet("s1b", "eu-west-1b"), subnet("s-noaz", ""),
			alb("alb-1sub"), alb("alb-samezone"), alb("alb-ok"), alb("alb-undeclared")},
		Edges: []core.CanvasEdge{
			in("v1", "s1a", "vpc"), in("v2", "s1a2", "vpc"), in("v3", "s1b", "vpc"), in("v4", "s-noaz", "vpc"),
			in("a1", "alb-1sub", "s1a"),
			in("b1", "alb-samezone", "s1a"), in("b2", "alb-samezone", "s1a2"),
			in("c1", "alb-ok", "s1a"), in("c2", "alb-ok", "s1b"),
			in("d1", "alb-undeclared", "s1a"), in("d2", "alb-undeclared", "s-noaz"),
		},
	}
	if got, want := rules(core.ValidateCanvasPlacement(doc)), "PLACE-ALB-TWO-AZS@alb-1sub,PLACE-ALB-TWO-AZS@alb-samezone"; got != want {
		t.Fatalf("%s (%s): got %q, want %q — see %s", spec.ID, spec.Rule, got, want, spec.Source)
	}
}

// The ALB rule is verified only for Application Load Balancers; a load balancer whose
// type is anything else, or not stated, must never be held to it.
func TestPlacement_ALBRuleDoesNotApplyToOtherOrUnstatedLBTypes_002(t *testing.T) {
	spec := harness.Verify(t, harness.Spec{
		ID:            "PLACE-ALB-TWO-AZS-002",
		Rule:          "The two-AZ requirement is documented for Application Load Balancers; this engine does not extend it to a load balancer of another or unstated type.",
		Source:        "https://docs.aws.amazon.com/elasticloadbalancing/latest/application/application-load-balancers.html",
		Scenario:      "An aws_lb in one subnet whose load_balancer_type is network, and one whose type is unstated.",
		Configuration: "lb-nlb (sizing load_balancer_type=network, one subnet), lb-untyped (no sizing, one subnet)",
		Request:       "ValidateCanvasPlacement over the document.",
		Expected:      "No violations — claiming one would be asserting AWS behaviour this rule's source does not state.",
	})
	nlb, untyped := alb("lb-nlb"), alb("lb-untyped")
	nlb.Sizing = map[string]string{"load_balancer_type": "network"}
	untyped.Sizing = nil
	doc := core.CanvasDocument{
		Nodes: []core.CanvasNode{node("vpc", "aws_vpc"), subnet("s1", "eu-west-1a"), nlb, untyped},
		Edges: []core.CanvasEdge{in("v1", "s1", "vpc"), in("a", "lb-nlb", "s1"), in("b", "lb-untyped", "s1")},
	}
	if got := rules(core.ValidateCanvasPlacement(doc)); got != "" {
		t.Fatalf("%s (%s): got violations %q, want none — see %s", spec.ID, spec.Rule, got, spec.Source)
	}
}

func TestPlacement_DBSubnetGroupNeedsTwoAvailabilityZones_001(t *testing.T) {
	spec := harness.Verify(t, harness.Spec{
		ID:            "PLACE-DBSUBNETGROUP-TWO-AZS-001",
		Rule:          "A DB subnet group must have at least one subnet in at least two Availability Zones in the AWS Region (\"Each DB subnet group must have at least one subnet in at least two Availability Zones\"). The Local Zone single-subnet exception is not modelled.",
		Source:        "https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/USER_VPC.WorkingWithRDSInstanceinaVPC.html",
		Scenario:      "A DB subnet group with subnets in one zone, and one with subnets in two zones.",
		Configuration: "dbsg-one (sa, sa2 both eu-west-1a), dbsg-two (sa, sb)",
		Request:       "ValidateCanvasPlacement over the document.",
		Expected:      "A violation for dbsg-one only.",
	})
	doc := core.CanvasDocument{
		Nodes: []core.CanvasNode{node("vpc", "aws_vpc"), subnet("sa", "eu-west-1a"), subnet("sa2", "eu-west-1a"), subnet("sb", "eu-west-1b"),
			node("dbsg-one", "aws_db_subnet_group"), node("dbsg-two", "aws_db_subnet_group")},
		Edges: []core.CanvasEdge{in("v1", "sa", "vpc"), in("v2", "sa2", "vpc"), in("v3", "sb", "vpc"),
			in("o1", "dbsg-one", "sa"), in("o2", "dbsg-one", "sa2"), in("t1", "dbsg-two", "sa"), in("t2", "dbsg-two", "sb")},
	}
	if got, want := rules(core.ValidateCanvasPlacement(doc)), "PLACE-DBSUBNETGROUP-TWO-AZS@dbsg-one"; got != want {
		t.Fatalf("%s (%s): got %q, want %q — see %s", spec.ID, spec.Rule, got, want, spec.Source)
	}
}

// Violations carry the documentation they come from and are sorted deterministically.
func TestPlacement_ViolationsCiteSourceAndAreDeterministic(t *testing.T) {
	doc := core.CanvasDocument{Nodes: []core.CanvasNode{subnet("b", "x"), subnet("a", "x")}}
	vs := core.ValidateCanvasPlacement(doc)
	if len(vs) != 2 || vs[0].ResourceID != "a" || vs[1].ResourceID != "b" {
		t.Fatalf("want two violations sorted a,b, got %+v", vs)
	}
	for _, v := range vs {
		if !strings.HasPrefix(v.Source, "https://docs.aws.amazon.com/") || v.Message == "" {
			t.Errorf("violation %+v must cite an AWS doc URL and a message", v)
		}
	}
}

const defaultNACLDoc = "https://docs.aws.amazon.com/vpc/latest/userguide/default-network-acl.html"

func TestPlacement_DefaultNACLBelongsToOneVPC_001(t *testing.T) {
	spec := harness.Verify(t, harness.Spec{
		ID:            "PLACE-DEFAULT-NACL-ONE-VPC-001",
		Rule:          "A VPC automatically comes with a default network ACL, so a default network ACL belongs to exactly one VPC.",
		Source:        defaultNACLDoc,
		Scenario:      "A default network ACL drawn inside no VPC, inside one VPC, and inside two VPCs.",
		Configuration: "d-none (no contained_in), d-one (in vpc1), d-two (in vpc1 and vpc2)",
		Request:       "ValidateCanvasPlacement over the document.",
		Expected:      "Violations for d-none and d-two only.",
	})
	doc := core.CanvasDocument{
		Nodes: []core.CanvasNode{node("vpc1", "aws_vpc"), node("vpc2", "aws_vpc"), node("d-none", "aws_default_network_acl"), node("d-one", "aws_default_network_acl"), node("d-two", "aws_default_network_acl")},
		Edges: []core.CanvasEdge{in("e1", "d-one", "vpc1"), in("e2", "d-two", "vpc1"), in("e3", "d-two", "vpc2")},
	}
	got := rules(core.ValidateCanvasPlacement(doc))
	for _, want := range []string{"PLACE-DEFAULT-NACL-ONE-VPC@d-none", "PLACE-DEFAULT-NACL-ONE-VPC@d-two"} {
		if !strings.Contains(got, want) {
			t.Errorf("%s (%s): missing %q in %q — see %s", spec.ID, spec.Rule, want, got, spec.Source)
		}
	}
	if strings.Contains(got, "PLACE-DEFAULT-NACL-ONE-VPC@d-one") {
		t.Errorf("%s: d-one is in exactly one VPC and must not be flagged: %q", spec.ID, got)
	}
}

func TestPlacement_VPCHasOneDefaultNACL_001(t *testing.T) {
	spec := harness.Verify(t, harness.Spec{
		ID:            "PLACE-DEFAULT-NACL-ONE-PER-VPC-001",
		Rule:          "A VPC automatically comes with a default network ACL (singular), so two default network ACLs declared for one VPC leave no way to know which one its unassociated subnets use.",
		Source:        defaultNACLDoc,
		Scenario:      "Two default network ACLs in one VPC, and one each in another.",
		Configuration: "da and db in vpc1; dc in vpc2",
		Request:       "ValidateCanvasPlacement over the document.",
		Expected:      "Violations for da and db only; dc, the only default in its VPC, is fine.",
	})
	doc := core.CanvasDocument{
		Nodes: []core.CanvasNode{node("vpc1", "aws_vpc"), node("vpc2", "aws_vpc"), node("da", "aws_default_network_acl"), node("db", "aws_default_network_acl"), node("dc", "aws_default_network_acl")},
		Edges: []core.CanvasEdge{in("e1", "da", "vpc1"), in("e2", "db", "vpc1"), in("e3", "dc", "vpc2")},
	}
	if got, want := rules(core.ValidateCanvasPlacement(doc)), "PLACE-DEFAULT-NACL-ONE-PER-VPC@da,PLACE-DEFAULT-NACL-ONE-PER-VPC@db"; got != want {
		t.Fatalf("%s (%s): got %q, want %q — see %s", spec.ID, spec.Rule, got, want, spec.Source)
	}
}
