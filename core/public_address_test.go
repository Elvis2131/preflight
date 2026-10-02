package core_test

// PC-156: public IPv4 assignment decides whether an outbound journey can use an internet gateway.
// Every case goes through real HCL ingest so the mapping data, the unresolved-attribute tracking
// and the Elastic IP edge are exercised, not just the resolver.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"preflight/core"
	"preflight/ingest"
	awsprovider "preflight/providers/aws"
)

const igwBundle = `
resource "aws_vpc" "v" { cidr_block = "10.0.0.0/16" }
resource "aws_internet_gateway" "igw" { vpc_id = aws_vpc.v.id }
resource "aws_subnet" "pub" {
  vpc_id            = aws_vpc.v.id
  cidr_block        = "10.0.1.0/24"
  availability_zone = "eu-west-1a"
  %SUBNET%
}
resource "aws_route_table" "rt" {
  vpc_id = aws_vpc.v.id
  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.igw.id
  }
}
resource "aws_route_table_association" "a" {
  subnet_id      = aws_subnet.pub.id
  route_table_id = aws_route_table.rt.id
}
resource "aws_security_group" "sg" {
  vpc_id = aws_vpc.v.id
  egress {
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
}
resource "aws_instance" "app" {
  subnet_id              = aws_subnet.pub.id
  vpc_security_group_ids = [aws_security_group.sg.id]
  %INSTANCE%
}
%EXTRA%
# Entry point and stateful node only so the bundle clears the minimum viable graph.
resource "aws_lb" "front" { name = "front" }
resource "aws_db_instance" "d" { identifier = "d" }
`

func igwIR(t *testing.T, subnetAttr, instanceAttr, extra string) *core.IR {
	t.Helper()
	dir := t.TempDir()
	src := strings.NewReplacer("%SUBNET%", subnetAttr, "%INSTANCE%", instanceAttr, "%EXTRA%", extra).Replace(igwBundle)
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	reg, err := awsprovider.Load()
	if err != nil {
		t.Fatal(err)
	}
	res, err := ingest.Ingest(dir, reg, 1)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if res.Insufficient != nil {
		t.Fatalf("insufficient_model: %+v", res.Insufficient)
	}
	return res.IR
}

func publicAddressStep(t *testing.T, tr core.Trace) core.TraceStep {
	t.Helper()
	for _, s := range tr.Steps {
		if s.Step == "public_address" {
			return s
		}
	}
	t.Fatalf("no public_address step in %+v", tr.Steps)
	return core.TraceStep{}
}

func TestOutboundViaInternetGateway_PublicAddress(t *testing.T) {
	cases := []struct {
		name                    string
		subnet, instance, extra string
		want                    core.TraceDecision
		wantAssumed             bool
		wantReasonContains      string
	}{
		{"subnet auto-assigns", `map_public_ip_on_launch = true`, ``, ``, core.TraceAllow, false, "auto-assigns"},
		{"subnet attribute undeclared: provider default false, assumed", ``, ``, ``, core.TraceDeny, true, "default (false)"},
		{"subnet declares false", `map_public_ip_on_launch = false`, ``, ``, core.TraceDeny, false, "= false"},
		{"launch setting true overrides subnet false", `map_public_ip_on_launch = false`, `associate_public_ip_address = true`, ``, core.TraceAllow, false, "launched with"},
		{"launch setting false overrides subnet true", `map_public_ip_on_launch = true`, `associate_public_ip_address = false`, ``, core.TraceDeny, false, "launched with"},
		{"Elastic IP wins over subnet false", `map_public_ip_on_launch = false`, ``,
			`resource "aws_eip" "e" {
  domain   = "vpc"
  instance = aws_instance.app.id
}`, core.TraceAllow, false, "Elastic IP"},
		{"Elastic IP wins over launch false", ``, `associate_public_ip_address = false`,
			`resource "aws_eip" "e" {
  domain   = "vpc"
  instance = aws_instance.app.id
}`, core.TraceAllow, false, "Elastic IP"},
		{"subnet setting is a variable: unknown, not absent", `map_public_ip_on_launch = var.public`, ``, ``, core.TraceNotAssessable, false, "does not resolve"},
		{"launch setting is a variable: unknown", `map_public_ip_on_launch = true`, `associate_public_ip_address = var.assign`, ``, core.TraceNotAssessable, false, "does not resolve"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ir := igwIR(t, c.subnet, c.instance, c.extra)
			tr := core.BuildOutboundTrace(ir, "aws_instance.app", "tcp", 443)
			step := publicAddressStep(t, tr)
			if step.Decision != c.want {
				t.Fatalf("public_address = %s (%s), want %s; steps=%+v", step.Decision, step.Reason, c.want, tr.Steps)
			}
			if !strings.Contains(step.Reason, c.wantReasonContains) {
				t.Errorf("reason %q does not contain %q", step.Reason, c.wantReasonContains)
			}
			if got := step.Provenance.Kind == core.KindAssumed; got != c.wantAssumed {
				t.Errorf("provenance kind = %s, want assumed=%v", step.Provenance.Kind, c.wantAssumed)
			}
			if c.want == core.TraceAllow && !tr.Allowed {
				t.Errorf("trace blocked after an allowed public_address: %s", tr.Concise)
			}
			if c.want != core.TraceAllow && tr.Allowed {
				t.Errorf("trace allowed although public_address was %s", c.want)
			}
		})
	}
}

// The journey-level result names the step, and the SG/NACL/route checks still apply after it.
func TestOutboundViaInternetGateway_JourneyFlow(t *testing.T) {
	j := core.DeclaredJourney{ID: "call-out", Path: []string{"aws_instance.app", "internet"}, Protocol: "tcp", Port: 443}

	open := core.ComputeJourneyFlow(igwIR(t, `map_public_ip_on_launch = true`, ``, ``), j, nil)
	if !open.Flows {
		t.Fatalf("blocked: %s", open.BlockedReason)
	}
	closed := core.ComputeJourneyFlow(igwIR(t, ``, ``, ``), j, nil)
	if closed.Flows || !strings.Contains(closed.BlockedReason, "public_address") {
		t.Errorf("flows=%v reason=%q, want blocked at public_address", closed.Flows, closed.BlockedReason)
	}

	// Port 22 is not in the SG's egress rules: still blocked at the security group even with a public address.
	ssh := j
	ssh.Port = 22
	if f := core.ComputeJourneyFlow(igwIR(t, `map_public_ip_on_launch = true`, ``, ``), ssh, nil); f.Flows {
		t.Error("port 22 flowed with only a 443 egress rule")
	}
}

// A resource whose mapping gives no public_address_model is never judged by the subnet attribute.
func TestResolvePublicIPv4_UnmodelledServiceIsNotAssessable(t *testing.T) {
	ir := igwIR(t, `map_public_ip_on_launch = true`, ``, ``)
	pa := core.ResolvePublicIPv4(ir, "aws_db_instance.d", "aws_subnet.pub")
	if pa.Decision != core.TraceNotAssessable || !strings.Contains(pa.Reason, "not modelled") {
		t.Errorf("got %s %q, want not_assessable naming that it is not modelled", pa.Decision, pa.Reason)
	}
}

// Negative control: the NAT route needs no public address on the source, so the golden
// payment-rail journey (private subnets, NAT) must be unaffected by this check.
func TestOutboundViaNAT_NeedsNoPublicAddress(t *testing.T) {
	tr := core.BuildOutboundTrace(realGoldenIR(t), "aws_eks_cluster.payments", "tcp", 443)
	if !tr.Allowed {
		t.Fatalf("golden NAT path blocked: %s", tr.Concise)
	}
	for _, s := range tr.Steps {
		if s.Step == "public_address" {
			t.Errorf("NAT route produced a public_address step: %+v", s)
		}
	}
}
