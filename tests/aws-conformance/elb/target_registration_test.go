package elb

import (
	"os"
	"path/filepath"
	"testing"

	"preflight/core"
	"preflight/ingest"
	awsprovider "preflight/providers/aws"
	"preflight/tests/aws-conformance/harness"
)

const targetGroupsDoc = "https://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-target-groups.html"

func lbIR(scope string, targets ...string) *core.IR {
	prov := core.NewProvenance(core.KindStated, "conformance")
	mk := func(id string, t core.NodeType, raw map[string]any) core.Node {
		return core.Node{ID: id, Type: t, Resolution: core.ResolutionKnown, Provenance: prov, RawAttributes: raw}
	}
	e := func(id string, ty core.EdgeType, from, to string, raw map[string]any) core.Edge {
		return core.Edge{ID: id, Type: ty, From: from, To: to, Resolution: core.ResolutionKnown, Provenance: prov, RawAttributes: raw}
	}
	rs := string(core.CapabilityRequestSimulation)
	ir := &core.IR{SchemaVersion: "1.4.0", VersionNumber: 1, VersionHash: "h", Nodes: []core.Node{
		mk("vpc", core.NodeTypeNetworkBoundary, map[string]any{"network_role": "vpc"}),
		mk("sA", core.NodeTypeNetworkBoundary, map[string]any{"network_role": "subnet", "cidr_block": "10.0.1.0/24"}),
		mk("sB", core.NodeTypeNetworkBoundary, map[string]any{"network_role": "subnet", "cidr_block": "10.0.2.0/24"}),
		mk("lb", core.NodeTypeLoadBalancer, map[string]any{"capability_level": rs}),
		mk("web", core.NodeTypeCompute, map[string]any{"capability_level": rs}),
		mk("other", core.NodeTypeCompute, map[string]any{"capability_level": rs}),
		mk("sgL", core.NodeTypeNetworkBoundary, map[string]any{"security_group_rules": []map[string]any{{"direction": "egress", "protocol": "-1", "cidr_blocks": []any{"0.0.0.0/0"}}}}),
		mk("sgW", core.NodeTypeNetworkBoundary, map[string]any{"security_group_rules": []map[string]any{{"direction": "ingress", "protocol": "tcp", "from_port": 8080, "to_port": 8080, "cidr_blocks": []any{"10.0.1.0/24"}}}}),
	}, Edges: []core.Edge{
		e("1", core.EdgeTypeContainedIn, "lb", "sA", nil), e("2", core.EdgeTypeContainedIn, "web", "sB", nil),
		e("3", core.EdgeTypeContainedIn, "other", "sB", nil), e("4", core.EdgeTypeContainedIn, "sA", "vpc", nil),
		e("5", core.EdgeTypeContainedIn, "sB", "vpc", nil), e("6", core.EdgeTypeDependsOn, "lb", "sgL", nil),
		e("7", core.EdgeTypeDependsOn, "web", "sgW", nil), e("8", core.EdgeTypeDependsOn, "other", "sgW", nil),
	}}
	for i, tgt := range targets {
		raw := map[string]any{}
		if scope != "" {
			raw["registration_scope"] = scope
		}
		ir.Edges = append(ir.Edges, e("t"+string(rune('a'+i)), core.EdgeTypeRoutesTo, "lb", tgt, raw))
	}
	return ir
}

func step(tr core.Trace) (core.TraceStep, bool) {
	for _, s := range tr.Steps {
		if s.Step == "target_registration" {
			return s, true
		}
	}
	return core.TraceStep{}, false
}

func TestElbTargetGroup_001_DeregisteredTargetReceivesNoRequests(t *testing.T) {
	spec := harness.Verify(t, harness.Spec{
		ID:            "ELB-TG-REG-001",
		Rule:          "Deregistering a target removes it from the target group, and the load balancer stops routing requests to a target as soon as it is deregistered.",
		Source:        targetGroupsDoc,
		Scenario:      "A load balancer whose only registered target is deregistered by a fault.",
		Configuration: "lb registers web (attachments-only edge); network path lb -> web is open on tcp/8080",
		Request:       "Trace lb -> web on tcp/8080, before and after the target_deregistration fault.",
		Expected:      "Before: target_registration allows and the trace is Allowed. After: target_registration denies and the trace is blocked there.",
	})
	ir := lbIR("attachments", "web")
	if tr := core.BuildTrace(ir, "lb", "web", "", "tcp", 8080); !tr.Allowed {
		t.Fatalf("%s: registered target must be reachable: %+v — see %s", spec.ID, tr.Steps, spec.Source)
	}
	mutated, ok := core.WithTargetDeregistered(ir, "lb", "web")
	if !ok {
		t.Fatalf("%s: deregistration refused for a real registration", spec.ID)
	}
	tr := core.BuildTrace(mutated, "lb", "web", "", "tcp", 8080)
	s, found := step(tr)
	if tr.Allowed || !found || s.Decision != core.TraceDeny {
		t.Errorf("%s (%s): deregistered target must be denied at target_registration, got allowed=%v step=%+v", spec.ID, spec.Rule, tr.Allowed, s)
	}
}

func TestElbTargetGroup_002_AttachmentsDoNotProveAbsence(t *testing.T) {
	spec := harness.Verify(t, harness.Spec{
		ID:            "ELB-TG-REG-002",
		Rule:          "If you register targets by instance ID, you can use your load balancer with an Auto Scaling group: after a target group is attached to an Auto Scaling group, Auto Scaling registers your targets with the target group for you when it launches them.",
		Source:        targetGroupsDoc,
		Scenario:      "A target group whose registered targets come from attachment resources, asked about an instance those resources do not list.",
		Configuration: "lb registers web through an attachment (attachments-only edge); other is not listed",
		Request:       "Trace lb -> other on tcp/8080.",
		Expected:      "target_registration is not_assessable (the instance may have been registered by an Auto Scaling group), never a deny. With canvas-authored (complete) registration the same unlisted instance IS denied.",
	})
	partial := core.BuildTrace(lbIR("attachments", "web"), "lb", "other", "", "tcp", 8080)
	if s, found := step(partial); !found || s.Decision != core.TraceNotAssessable {
		t.Errorf("%s (%s): unlisted target under attachments-only registration must be not_assessable, got %+v found=%v", spec.ID, spec.Rule, s, found)
	}
	complete := core.BuildTrace(lbIR("", "web"), "lb", "other", "", "tcp", 8080)
	if s, found := step(complete); !found || s.Decision != core.TraceDeny {
		t.Errorf("%s: unlisted target under complete registration must be denied, got %+v found=%v", spec.ID, s, found)
	}
}

func TestElbTargetGroup_003_TargetTypeDeterminesWhatIsRegistered(t *testing.T) {
	spec := harness.Verify(t, harness.Spec{
		ID:            "ELB-TG-TYPE-001",
		Rule:          "A target group's target type determines the type of target you specify when registering targets: an instance ID for type instance, an IP address for type ip, a Lambda function for type lambda.",
		Source:        targetGroupsDoc,
		Scenario:      "The same attachment against target groups of different types.",
		Configuration: "a listener forwards to a target group; an attachment names an aws_instance as target_id",
		Request:       "Ingest the bundle with target_type unset (instance), \"ip\", and \"lambda\".",
		Expected:      "Only the instance-type group yields a load balancer -> instance registration; an ip group's target is an address that identifies no node, and a lambda group registers a function, not an instance.",
	})
	ingestEdges := func(targetType string) int {
		dir := t.TempDir()
		tt := ""
		if targetType != "" {
			tt = `target_type = "` + targetType + `"`
		}
		src := `
resource "aws_vpc" "v" { cidr_block = "10.0.0.0/16" }
resource "aws_subnet" "s" {
  vpc_id     = aws_vpc.v.id
  cidr_block = "10.0.1.0/24"
}
resource "aws_lb" "front" {
  name    = "front"
  subnets = [aws_subnet.s.id]
}
resource "aws_instance" "web" { subnet_id = aws_subnet.s.id }
resource "aws_db_instance" "d" { identifier = "d" }
resource "aws_lb_target_group" "tg" {
  port     = 8080
  protocol = "HTTP"
  vpc_id   = aws_vpc.v.id
  ` + tt + `
}
resource "aws_lb_listener" "l" {
  load_balancer_arn = aws_lb.front.arn
  port              = 80
  protocol          = "HTTP"
  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.tg.arn
  }
}
resource "aws_lb_target_group_attachment" "a" {
  target_group_arn = aws_lb_target_group.tg.arn
  target_id        = aws_instance.web.id
}
`
		if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		reg, err := awsprovider.Load()
		if err != nil {
			t.Fatal(err)
		}
		res, err := ingest.Ingest(dir, reg, 1)
		if err != nil || res.IR == nil {
			t.Fatalf("%s: ingest: %v %+v", spec.ID, err, res.Insufficient)
		}
		n := 0
		for _, e := range res.IR.Edges {
			if e.Type == core.EdgeTypeRoutesTo && e.From == "aws_lb.front" {
				n++
			}
		}
		return n
	}
	for typ, want := range map[string]int{"": 1, "instance": 1, "ip": 0, "lambda": 0} {
		if got := ingestEdges(typ); got != want {
			t.Errorf("%s (%s): target_type %q produced %d registration edges, want %d — see %s", spec.ID, spec.Rule, typ, got, want, spec.Source)
		}
	}
}
