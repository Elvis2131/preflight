package core_test

// PC-150: load balancer target registration read from HCL. golden/aws registers its targets through
// a controller and has no aws_lb_target_group_attachment, so these cases run on a SYNTHETIC
// Terraform bundle, stated as such, ingested through the real HCL path.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"preflight/core"
	"preflight/ingest"
	awsprovider "preflight/providers/aws"
)

const lbBundle = `
resource "aws_vpc" "v" { cidr_block = "10.0.0.0/16" }
resource "aws_subnet" "lbnet" {
  vpc_id            = aws_vpc.v.id
  cidr_block        = "10.0.1.0/24"
  availability_zone = "eu-west-1a"
}
resource "aws_subnet" "appnet" {
  vpc_id            = aws_vpc.v.id
  cidr_block        = "10.0.2.0/24"
  availability_zone = "eu-west-1a"
}
resource "aws_security_group" "lb" {
  vpc_id = aws_vpc.v.id
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}
resource "aws_security_group" "app" {
  vpc_id = aws_vpc.v.id
  ingress {
    from_port   = 8080
    to_port     = 8080
    protocol    = "tcp"
    cidr_blocks = ["10.0.1.0/24"]
  }
}
resource "aws_lb" "front" {
  name            = "front"
  subnets         = [aws_subnet.lbnet.id]
  security_groups = [aws_security_group.lb.id]
}
resource "aws_instance" "web" {
  subnet_id              = aws_subnet.appnet.id
  vpc_security_group_ids = [aws_security_group.app.id]
}
resource "aws_instance" "other" {
  subnet_id              = aws_subnet.appnet.id
  vpc_security_group_ids = [aws_security_group.app.id]
}
resource "aws_db_instance" "d" { identifier = "d" }
%EXTRA%
`

const forwardListener = `
resource "aws_lb_listener" "https" {
  load_balancer_arn = aws_lb.front.arn
  port              = 443
  protocol          = "HTTP"
  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.tg.arn
  }
}
`

func tgBlock(targetType string) string {
	extra := ""
	if targetType != "" {
		extra = `target_type = ` + targetType
	}
	return `
resource "aws_lb_target_group" "tg" {
  port     = 8080
  protocol = "HTTP"
  vpc_id   = aws_vpc.v.id
  ` + extra + `
}
`
}

func hclLBBundleIR(t *testing.T, extra string) *core.IR {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(strings.Replace(lbBundle, "%EXTRA%", extra, 1)), 0o644); err != nil {
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

func lbTargets(ir *core.IR, lb string) []string {
	var out []string
	for _, e := range ir.Edges {
		if e.Type == core.EdgeTypeRoutesTo && e.From == lb {
			out = append(out, e.To)
		}
	}
	return out
}

func registrationStep(tr core.Trace) (core.TraceStep, bool) {
	for _, s := range tr.Steps {
		if s.Step == "target_registration" {
			return s, true
		}
	}
	return core.TraceStep{}, false
}

const attachWeb = `
resource "aws_lb_target_group_attachment" "web" {
  target_group_arn = aws_lb_target_group.tg.arn
  target_id        = aws_instance.web.id
}
`

func TestLBTargets_HCL_ListenerTargetGroupAttachmentBecomesEdge(t *testing.T) {
	ir := hclLBBundleIR(t, forwardListener+tgBlock("")+attachWeb)
	got := lbTargets(ir, "aws_lb.front")
	if len(got) != 1 || got[0] != "aws_instance.web" {
		t.Fatalf("aws_lb.front targets = %v, want [aws_instance.web]", got)
	}
	for _, e := range ir.Edges {
		if e.From == "aws_lb.front" && e.Type == core.EdgeTypeRoutesTo {
			if e.RawAttributes["registration_scope"] != "attachments" || e.RawAttributes["destination_cidr"] != nil {
				t.Errorf("edge attrs = %v, want registration_scope=attachments and no destination_cidr", e.RawAttributes)
			}
			if e.Provenance.Kind != core.KindStated {
				t.Errorf("provenance = %s, want stated", e.Provenance.Kind)
			}
		}
	}
}

func TestLBTargets_HCL_NoEdgeWithoutAForwardOrAnAttachment(t *testing.T) {
	cases := map[string]string{
		"listener and target group, no attachment":  forwardListener + tgBlock(""),
		"attachment but no listener forwards to it": tgBlock("") + attachWeb,
		"listener redirects, does not forward": `
resource "aws_lb_listener" "https" {
  load_balancer_arn = aws_lb.front.arn
  port              = 80
  protocol          = "HTTP"
  default_action {
    type = "redirect"
    redirect {
      port        = "443"
      protocol    = "HTTPS"
      status_code = "HTTP_301"
    }
  }
}` + tgBlock("") + attachWeb,
		"ip target type: an address is not a node": forwardListener + tgBlock(`"ip"`) + attachWeb,
		"target_type is a variable":                forwardListener + tgBlock("var.type") + attachWeb,
		"lambda target group, instance attached":   forwardListener + tgBlock(`"lambda"`) + attachWeb,
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			if got := lbTargets(hclLBBundleIR(t, extra), "aws_lb.front"); len(got) != 0 {
				t.Errorf("targets = %v, want none", got)
			}
		})
	}
}

func TestLBTargets_HCL_ListenerRuleForwardAndLambdaTarget(t *testing.T) {
	rule := `
resource "aws_lb_listener" "https" {
  load_balancer_arn = aws_lb.front.arn
  port              = 443
  protocol          = "HTTP"
  default_action {
    type = "fixed-response"
  }
}
resource "aws_lb_listener_rule" "r" {
  listener_arn = aws_lb_listener.https.arn
  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.tg.arn
  }
}
`
	if got := lbTargets(hclLBBundleIR(t, rule+tgBlock("")+attachWeb), "aws_lb.front"); len(got) != 1 || got[0] != "aws_instance.web" {
		t.Errorf("listener-rule forward targets = %v, want [aws_instance.web]", got)
	}
	fn := `resource "aws_lambda_function" "fn" { function_name = "fn" }
resource "aws_lb_target_group_attachment" "fn" {
  target_group_arn = aws_lb_target_group.tg.arn
  target_id        = aws_lambda_function.fn.arn
}`
	if got := lbTargets(hclLBBundleIR(t, forwardListener+tgBlock(`"lambda"`)+fn), "aws_lb.front"); len(got) != 1 || got[0] != "aws_lambda_function.fn" {
		t.Errorf("lambda targets = %v, want [aws_lambda_function.fn]", got)
	}
}

func TestLBTargets_HCL_GoldenAwsStillYieldsNone(t *testing.T) {
	for _, e := range realGoldenIR(t).Edges {
		if e.Type == core.EdgeTypeRoutesTo && e.From == "aws_lb.payments" {
			t.Errorf("golden/aws registers through a controller; found a registration edge %+v", e)
		}
	}
}

func TestLBTargets_HCL_TraceNamesRegistrationAndDeregistrationBreaksJourney(t *testing.T) {
	ir := hclLBBundleIR(t, forwardListener+tgBlock("")+attachWeb)
	before := core.BuildTrace(ir, "aws_lb.front", "aws_instance.web", "", "tcp", 8080)
	if !before.Allowed {
		t.Fatalf("baseline blocked: %s", before.Concise)
	}
	if s, ok := registrationStep(before); !ok || s.Decision != core.TraceAllow {
		t.Fatalf("baseline target_registration = %+v ok=%v, want an allowing step", s, ok)
	}

	wl := core.Workload{Journeys: []core.DeclaredJourney{{ID: "j", Name: "j", Path: []string{"aws_lb.front", "aws_instance.web"}, Protocol: "tcp", Port: 8080, Criticality: "tier1"}}}
	after := core.Simulate(ir, wl, []core.Fault{{Type: "target_deregistration", Target: "aws_lb.front", DeregisterTarget: "aws_instance.web"}}, core.NewProvenance(core.KindDerived, "test"))
	if len(after.FlowDetail) != 1 || after.FlowDetail[0].Flows {
		t.Fatalf("after deregistration the journey must not flow: %+v", after.FlowDetail)
	}
	mutated, ok := core.WithTargetDeregistered(ir, "aws_lb.front", "aws_instance.web")
	if !ok {
		t.Fatal("WithTargetDeregistered refused a real HCL registration")
	}
	tr := core.BuildTrace(mutated, "aws_lb.front", "aws_instance.web", "", "tcp", 8080)
	s, found := registrationStep(tr)
	if tr.Allowed || !found || s.Decision != core.TraceDeny {
		t.Errorf("after deregistration: allowed=%v step=%+v found=%v, want a denying target_registration step", tr.Allowed, s, found)
	}
}

// Attachments prove registration; their absence proves nothing (Auto Scaling, ECS and controllers
// also register targets). A target the attachments do not list is not_assessable, never a deny, and
// the deregistration fault refuses it rather than guessing.
func TestLBTargets_HCL_UnlistedTargetIsNotAssessableAndFaultRefuses(t *testing.T) {
	ir := hclLBBundleIR(t, forwardListener+tgBlock("")+attachWeb)
	tr := core.BuildTrace(ir, "aws_lb.front", "aws_instance.other", "", "tcp", 8080)
	s, found := registrationStep(tr)
	if !found || s.Decision != core.TraceNotAssessable {
		t.Fatalf("unlisted target: step=%+v found=%v, want not_assessable", s, found)
	}
	if !strings.Contains(s.Reason, "Auto Scaling") {
		t.Errorf("reason %q should name the other ways targets get registered", s.Reason)
	}

	wl := core.Workload{Journeys: []core.DeclaredJourney{{ID: "j", Name: "j", Path: []string{"aws_lb.front", "aws_instance.other"}, Protocol: "tcp", Port: 8080, Criticality: "tier1"}}}
	res := core.Simulate(ir, wl, []core.Fault{{Type: "target_deregistration", Target: "aws_lb.front", DeregisterTarget: "aws_instance.other"}}, core.NewProvenance(core.KindDerived, "test"))
	if len(res.FlowDetail) != 0 {
		t.Errorf("fault on an unlisted target must be refused, got flow detail %+v", res.FlowDetail)
	}
}

// No registration facts at all (the golden shape): the trace makes no registration claim.
func TestLBTargets_HCL_NoAttachmentsMeansNoClaim(t *testing.T) {
	ir := hclLBBundleIR(t, forwardListener+tgBlock(""))
	tr := core.BuildTrace(ir, "aws_lb.front", "aws_instance.web", "", "tcp", 8080)
	if _, found := registrationStep(tr); found {
		t.Error("a load balancer with no registration facts produced a target_registration step")
	}
	if !tr.Allowed {
		t.Errorf("trace blocked without registration facts: %s", tr.Concise)
	}
}
