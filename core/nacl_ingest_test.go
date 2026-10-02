package core_test

// PC-158: NACL ingest. An inline ingress{}/egress{} block uses rule_no and action (provider docs), the standalone
// aws_network_acl_rule uses rule_number and rule_action; and an inline subnet_ids is an association. Reading the
// standalone names for both shapes ingested every inline rule with no number and no action, and the inline
// association was never recognised, so a NACL that denied everything read as allowing everything.

import (
	"strings"
	"testing"

	"preflight/core"
)

const naclBundle = `
resource "aws_vpc" "v" { cidr_block = "10.0.0.0/16" }
resource "aws_subnet" "s" {
  vpc_id     = aws_vpc.v.id
  cidr_block = "10.0.1.0/24"
}
%NACL%
`

func naclIR(t *testing.T, nacl string) *core.IR {
	t.Helper()
	return iamBundleIR(t, strings.Replace(naclBundle, "%NACL%", nacl, 1))
}

func TestNACL_InlineRuleKeepsItsNumberAndAction_AndInlineAssociationIsRecognised(t *testing.T) {
	ir := naclIR(t, `
resource "aws_network_acl" "deny" {
  vpc_id     = aws_vpc.v.id
  subnet_ids = [aws_subnet.s.id]
  ingress {
    rule_no    = 100
    action     = "deny"
    protocol   = "-1"
    cidr_block = "0.0.0.0/0"
    from_port  = 0
    to_port    = 0
  }
}`)
	res, ok := core.ResolveSubnetNACL(ir.Nodes, ir.Edges, "aws_subnet.s")
	if !ok || res.Source != core.NACLExplicit || res.Profile.NACLID != "aws_network_acl.deny" {
		t.Fatalf("the inline subnet_ids association must resolve to the NACL, got %+v ok=%v (it used to fall back to the assumed allow-all default)", res, ok)
	}
	if len(res.Profile.Rules) != 1 || res.Profile.Rules[0].Number != 100 || res.Profile.Rules[0].Allow {
		t.Errorf("rules = %+v, want one rule numbered 100 that denies (rule_no and action were ignored)", res.Profile.Rules)
	}
	for _, e := range ir.Edges {
		if e.Type == core.EdgeTypeContainedIn && e.From == "aws_network_acl.deny" && e.To == "aws_subnet.s" {
			t.Error("a NACL is not contained in a subnet: the generic reference walker's edge must not be emitted")
		}
	}
}

// An unreadable rule field, owner or association makes the subnet's NACL not_assessable, never the assumed default.
func TestNACL_UnreadableValue_IsNotAssessable_NeverTheAssumedDefault(t *testing.T) {
	cases := map[string]string{
		"inline rule action from a variable": `
resource "aws_network_acl" "n" {
  vpc_id     = aws_vpc.v.id
  subnet_ids = [aws_subnet.s.id]
  ingress {
    rule_no    = 100
    action     = var.action
    protocol   = "-1"
    cidr_block = "0.0.0.0/0"
    from_port  = 0
    to_port    = 0
  }
}`,
		"inline rule cidr from a reference": `
resource "aws_network_acl" "n" {
  vpc_id     = aws_vpc.v.id
  subnet_ids = [aws_subnet.s.id]
  ingress {
    rule_no    = 100
    action     = "allow"
    protocol   = "-1"
    cidr_block = aws_vpc.v.cidr_block
    from_port  = 0
    to_port    = 0
  }
}`,
		"inline rule uses an unmodelled field": `
resource "aws_network_acl" "n" {
  vpc_id     = aws_vpc.v.id
  subnet_ids = [aws_subnet.s.id]
  ingress {
    rule_no         = 100
    action          = "allow"
    protocol        = "icmp"
    cidr_block      = "0.0.0.0/0"
    icmp_type       = 8
    icmp_code       = 0
    from_port       = 0
    to_port         = 0
  }
}`,
		"inline subnet_ids unreadable": `
resource "aws_network_acl" "n" {
  vpc_id     = aws_vpc.v.id
  subnet_ids = var.subnets
}`,
		"standalone rule direction unreadable": `
resource "aws_network_acl" "n" { vpc_id = aws_vpc.v.id }
resource "aws_network_acl_association" "a" {
  network_acl_id = aws_network_acl.n.id
  subnet_id      = aws_subnet.s.id
}
resource "aws_network_acl_rule" "r" {
  network_acl_id = aws_network_acl.n.id
  rule_number    = 100
  egress         = var.egress
  protocol       = "-1"
  rule_action    = "allow"
  cidr_block     = "0.0.0.0/0"
}`,
		"standalone rule owner unreadable": `
resource "aws_network_acl" "n" { vpc_id = aws_vpc.v.id }
resource "aws_network_acl_association" "a" {
  network_acl_id = aws_network_acl.n.id
  subnet_id      = aws_subnet.s.id
}
resource "aws_network_acl_rule" "r" {
  network_acl_id = var.acl
  rule_number    = 100
  egress         = false
  protocol       = "-1"
  rule_action    = "deny"
  cidr_block     = "0.0.0.0/0"
}`,
		"association resource endpoint unreadable": `
resource "aws_network_acl" "n" { vpc_id = aws_vpc.v.id }
resource "aws_network_acl_association" "a" {
  network_acl_id = aws_network_acl.n.id
  subnet_id      = var.subnet
}`,
	}
	for name, tf := range cases {
		t.Run(name, func(t *testing.T) {
			ir := naclIR(t, tf)
			if res, ok := core.ResolveSubnetNACL(ir.Nodes, ir.Edges, "aws_subnet.s"); ok {
				t.Errorf("resolved to %+v (source %s): an unreadable NACL input must make it not_assessable, not a verdict or the assumed default", res.Profile.NACLID, res.Source)
			}
		})
	}
}

// An address the rules only partly cover (here: a source subnet whose CIDR could not be read, against a rule for
// 10.0.0.0/16) cannot be decided. The evaluators said so only inside a reason string, and the trace turned
// Allowed=false into a DENIAL; it is not_assessable (PC-158).
func TestTrace_AmbiguousSourceAddress_IsNotAssessableNeverADenial(t *testing.T) {
	bundle := func(srcCIDR string) *core.IR {
		return iamBundleIR(t, `
resource "aws_vpc" "v" { cidr_block = "10.0.0.0/16" }
resource "aws_subnet" "a" {
  vpc_id     = aws_vpc.v.id
  cidr_block = `+srcCIDR+`
}
resource "aws_subnet" "b" {
  vpc_id     = aws_vpc.v.id
  cidr_block = "10.0.2.0/24"
}
resource "aws_security_group" "a" {
  vpc_id = aws_vpc.v.id
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}
resource "aws_security_group" "b" {
  vpc_id = aws_vpc.v.id
  ingress {
    from_port   = 8080
    to_port     = 8080
    protocol    = "tcp"
    cidr_blocks = ["10.0.0.0/16"]
  }
}
resource "aws_instance" "a" {
  subnet_id              = aws_subnet.a.id
  vpc_security_group_ids = [aws_security_group.a.id]
}
resource "aws_instance" "b" {
  subnet_id              = aws_subnet.b.id
  vpc_security_group_ids = [aws_security_group.b.id]
}`)
	}
	cases := map[string]struct {
		cidr string
		want core.TraceDecision
	}{
		"source inside the rule's range":  {`"10.0.1.0/24"`, core.TraceAllow},
		"source outside the rule's range": {`"192.168.0.0/24"`, core.TraceDeny},
		"source CIDR could not be read":   {`var.cidr`, core.TraceNotAssessable},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			tr := core.BuildTrace(bundle(c.cidr), "aws_instance.a", "aws_instance.b", "", "tcp", 8080)
			var got core.TraceDecision = core.TraceAllow
			for _, s := range tr.Steps {
				if s.Decision != core.TraceAllow {
					got = s.Decision
					break
				}
			}
			if got != c.want {
				t.Errorf("decision = %s, want %s (%s)", got, c.want, tr.Concise)
			}
		})
	}
}
