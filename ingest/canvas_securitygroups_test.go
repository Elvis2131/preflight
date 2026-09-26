package ingest_test

// PC-137: canvas-authored Security Group rules round-trip through
// contracts/canvas.schema.json into the exact same IR shape (RawAttributes
// ["security_group_rules"]) the Terraform path already produces — core.
// SecurityGroupProfile/EvaluateConnection reads it back identically regardless of
// which producer built it, never a second SG evaluation path.

import (
	"testing"

	"preflight/core"
	"preflight/ingest"
	"preflight/providers"
)

// buildCanvasSGDoc is one minimal, real canvas document: a load_balancer entry point
// (satisfies CheckMVG) and a managed_database with a security group attached via the
// SAME depends_on edge convention the Terraform path already uses. No subnets at
// all — deliberately, so BuildTrace's own route_selection/nacl_check steps (both
// skipped for a subnet-less, internet-originated request) never enter into this
// test, keeping it focused on the one thing PC-137 actually changed: the SG step.
func buildCanvasSGDoc(dbRules []core.CanvasSecurityGroupRule) core.CanvasDocument {
	return core.CanvasDocument{
		Nodes: []core.CanvasNode{
			{ID: "lb", Type: "load_balancer", Label: "LB", Capability: map[string]string{}, ServiceID: "aws_lb"},
			{ID: "db", Type: "managed_database", Label: "DB", Capability: map[string]string{}, ServiceID: "aws_db_instance"},
			{ID: "sg-db", Type: "network_boundary", Label: "DB SG", Capability: map[string]string{}, SecurityGroupRules: dbRules},
		},
		Edges: []core.CanvasEdge{
			{ID: "e1", Type: "depends_on", From: "lb", To: "db"},
			{ID: "e2", Type: "depends_on", From: "db", To: "sg-db"},
		},
	}
}

func TestIngestCanvas_SecurityGroupRules_RoundTripIntoRealIRShape(t *testing.T) {
	doc := buildCanvasSGDoc([]core.CanvasSecurityGroupRule{
		{Direction: "ingress", Protocol: "tcp", FromPort: 5432, ToPort: 5432, CIDRBlocks: []string{"0.0.0.0/0"}},
	})
	result, err := ingest.IngestCanvas(doc, providers.Registry(loadRegistry(t)), 1)
	if err != nil {
		t.Fatalf("IngestCanvas: %v", err)
	}
	if result.Insufficient != nil {
		t.Fatalf("unexpectedly insufficient: %+v", result.Insufficient)
	}

	var sgNode core.Node
	for _, n := range result.IR.Nodes {
		if n.ID == "sg-db" {
			sgNode = n
		}
	}
	rules, ok := sgNode.RawAttributes["security_group_rules"].([]map[string]any)
	if !ok || len(rules) != 1 {
		t.Fatalf("sg-db RawAttributes[security_group_rules] = %#v, want one real rule map", sgNode.RawAttributes["security_group_rules"])
	}
	// Exactly the shape ingest/securitygroups.go's own normalizeSGRule produces —
	// core.SecurityGroupProfile reads this back with toSGRule unchanged either way.
	if rules[0]["direction"] != "ingress" || rules[0]["protocol"] != "tcp" || rules[0]["from_port"] != 5432 || rules[0]["to_port"] != 5432 {
		t.Errorf("rule = %+v, want the real authored fields", rules[0])
	}

	// SecurityGroupProfile (core.go, unmodified) reads it back identically to a
	// Terraform-sourced rule — proven by actually calling it, not asserting the
	// RawAttributes shape alone.
	profile := core.SecurityGroupProfile(result.IR.Nodes, result.IR.Edges, "db")
	if len(profile.SGIDs) != 1 || profile.SGIDs[0] != "sg-db" || len(profile.Rules) != 1 {
		t.Fatalf("SecurityGroupProfile(db) = %+v, want one attached SG with one rule", profile)
	}
}

func TestBuildTrace_CanvasAuthoredSG_MatchingRule_Allows(t *testing.T) {
	doc := buildCanvasSGDoc([]core.CanvasSecurityGroupRule{
		{Direction: "ingress", Protocol: "tcp", FromPort: 5432, ToPort: 5432, CIDRBlocks: []string{"0.0.0.0/0"}},
	})
	result, err := ingest.IngestCanvas(doc, providers.Registry(loadRegistry(t)), 1)
	if err != nil {
		t.Fatalf("IngestCanvas: %v", err)
	}

	tr := core.BuildTrace(result.IR, "", "db", "0.0.0.0/0", "tcp", 5432)
	if !tr.Allowed {
		t.Fatalf("got denied, want allowed by the canvas-authored SG rule: %+v", tr.Steps)
	}
}

// TestBuildTrace_CanvasAuthoredSG_RemovingTheAllowingRule_FailsNamingTheRule is
// PC-137's own third acceptance criterion, verbatim: "removing the one allowing rule
// makes it fail at that step with the rule named."
func TestBuildTrace_CanvasAuthoredSG_RemovingTheAllowingRule_FailsNamingTheRule(t *testing.T) {
	// The same SG, but its one rule now permits a different port — the real-world
	// "someone tightened the SG rule" scenario PC-99's own Failure Lab names.
	doc := buildCanvasSGDoc([]core.CanvasSecurityGroupRule{
		{Direction: "ingress", Protocol: "tcp", FromPort: 22, ToPort: 22, CIDRBlocks: []string{"0.0.0.0/0"}},
	})
	result, err := ingest.IngestCanvas(doc, providers.Registry(loadRegistry(t)), 1)
	if err != nil {
		t.Fatalf("IngestCanvas: %v", err)
	}

	tr := core.BuildTrace(result.IR, "", "db", "0.0.0.0/0", "tcp", 5432)
	if tr.Allowed {
		t.Fatal("got allowed, want denied — the rule permits port 22, not 5432")
	}
	var last core.TraceStep
	for _, s := range tr.Steps {
		last = s
	}
	if last.Step != "sg_dest_ingress" || last.Decision != core.TraceDeny {
		t.Fatalf("got last step %+v, want a denying sg_dest_ingress step", last)
	}
	if last.Component != "db" {
		t.Errorf("Component = %q, want db (the resource whose own SG denied this)", last.Component)
	}
}

// TestParity_CanvasVsTerraform_SecurityGroupReferenceRule is PC-137's own second
// acceptance criterion: "an SG configuration authored on canvas and the equivalent
// Terraform produce identical PC-112 decisions." Reproduces golden/aws's own real
// database security group rule (golden/aws/security.tf: the "database" SG's inline
// ingress rule permits tcp:5432 FROM the "workload" SG by reference, not by CIDR) as
// an equivalent canvas document, and confirms both producers reach the same Allow/
// Deny decision for a matching request and the same decision for a non-matching one.
func TestParity_CanvasVsTerraform_SecurityGroupReferenceRule(t *testing.T) {
	reg := loadRegistry(t)

	tfResult, err := ingest.Ingest(findGoldenAWSDir(t), reg, 1)
	if err != nil {
		t.Fatalf("Ingest (Terraform path): %v", err)
	}
	if tfResult.Insufficient != nil {
		t.Fatalf("golden/aws unexpectedly insufficient: %+v", tfResult.Insufficient)
	}
	tfWorkloadProfile := core.SecurityGroupProfile(tfResult.IR.Nodes, tfResult.IR.Edges, "aws_eks_cluster.payments")
	tfDBProfile := core.SecurityGroupProfile(tfResult.IR.Nodes, tfResult.IR.Edges, "aws_db_instance.payments")
	if len(tfWorkloadProfile.SGIDs) == 0 || len(tfDBProfile.SGIDs) == 0 {
		t.Fatalf("golden/aws fixture drifted: expected both the workload and database resources to have a resolvable attached SG (workload=%v, db=%v)", tfWorkloadProfile.SGIDs, tfDBProfile.SGIDs)
	}

	canvasDoc := core.CanvasDocument{
		Nodes: []core.CanvasNode{
			{ID: "lb", Type: "load_balancer", Label: "LB", Capability: map[string]string{}, ServiceID: "aws_lb"},
			{ID: "workload", Type: "container_workload", Label: "Workload", Capability: map[string]string{}},
			{ID: "db", Type: "managed_database", Label: "DB", Capability: map[string]string{}, ServiceID: "aws_db_instance"},
			{
				ID: "sg-workload", Type: "network_boundary", Label: "Workload SG", Capability: map[string]string{},
				// Mirrors golden/aws/security.tf's own standalone
				// aws_security_group_rule "workload_to_database": the workload SG's
				// own EGRESS rule permitting 5432 out to the database SG by
				// reference — SG connections are evaluated on BOTH the initiator's
				// egress AND the responder's ingress (core.EvaluateConnection),
				// omitting this half is exactly the kind of incomplete replica this
				// parity test exists to catch.
				SecurityGroupRules: []core.CanvasSecurityGroupRule{
					{Direction: "egress", Protocol: "tcp", FromPort: 5432, ToPort: 5432, SourceSecurityGroup: "sg-db"},
				},
			},
			{
				ID: "sg-db", Type: "network_boundary", Label: "DB SG", Capability: map[string]string{},
				SecurityGroupRules: []core.CanvasSecurityGroupRule{
					{Direction: "ingress", Protocol: "tcp", FromPort: 5432, ToPort: 5432, SourceSecurityGroup: "sg-workload"},
				},
			},
		},
		Edges: []core.CanvasEdge{
			{ID: "e1", Type: "depends_on", From: "lb", To: "workload"},
			{ID: "e2", Type: "depends_on", From: "workload", To: "db"},
			{ID: "e3", Type: "depends_on", From: "workload", To: "sg-workload"},
			{ID: "e4", Type: "depends_on", From: "db", To: "sg-db"},
		},
	}
	canvasResult, err := ingest.IngestCanvas(canvasDoc, reg, 1)
	if err != nil {
		t.Fatalf("IngestCanvas: %v", err)
	}
	if canvasResult.Insufficient != nil {
		t.Fatalf("unexpectedly insufficient: %+v", canvasResult.Insufficient)
	}

	tfAllowed, _, _ := core.EvaluateConnection(tfWorkloadProfile, tfDBProfile, "10.0.0.0/16", "10.0.0.0/16", "tcp", 5432)
	canvasWorkloadProfile := core.SecurityGroupProfile(canvasResult.IR.Nodes, canvasResult.IR.Edges, "workload")
	canvasDBProfile := core.SecurityGroupProfile(canvasResult.IR.Nodes, canvasResult.IR.Edges, "db")
	canvasAllowed, _, _ := core.EvaluateConnection(canvasWorkloadProfile, canvasDBProfile, "10.0.0.0/16", "10.0.0.0/16", "tcp", 5432)

	if !tfAllowed {
		t.Fatal("Terraform path: expected tcp:5432 workload->db to be allowed by golden/aws's own real rule")
	}
	if canvasAllowed != tfAllowed {
		t.Errorf("parity broken: Terraform allowed=%v, canvas allowed=%v for the matching request (tcp:5432)", tfAllowed, canvasAllowed)
	}

	// Negative half of the same parity check: a non-matching port must be denied
	// identically by both producers too — parity on the allow case alone would be
	// vacuous (a bug that always returns true would "pass" it).
	tfAllowedWrongPort, _, _ := core.EvaluateConnection(tfWorkloadProfile, tfDBProfile, "10.0.0.0/16", "10.0.0.0/16", "tcp", 9999)
	canvasAllowedWrongPort, _, _ := core.EvaluateConnection(canvasWorkloadProfile, canvasDBProfile, "10.0.0.0/16", "10.0.0.0/16", "tcp", 9999)
	if tfAllowedWrongPort {
		t.Fatal("Terraform path: expected tcp:9999 to be denied — golden/aws's own rule only permits 5432")
	}
	if canvasAllowedWrongPort != tfAllowedWrongPort {
		t.Errorf("parity broken: Terraform allowed=%v, canvas allowed=%v for the non-matching request (tcp:9999)", tfAllowedWrongPort, canvasAllowedWrongPort)
	}
}

// TestBuildTrace_CanvasNode_NoSGAttached_NotAssessable is PC-137's own fourth
// acceptance criterion: "canvas node with no SG attached still yields
// not_assessable at the SG step, not deny or allow."
func TestBuildTrace_CanvasNode_NoSGAttached_NotAssessable(t *testing.T) {
	doc := core.CanvasDocument{
		Nodes: []core.CanvasNode{
			{ID: "lb", Type: "load_balancer", Label: "LB", Capability: map[string]string{}, ServiceID: "aws_lb"},
			{ID: "db", Type: "managed_database", Label: "DB", Capability: map[string]string{}, ServiceID: "aws_db_instance"},
		},
		Edges: []core.CanvasEdge{
			{ID: "e1", Type: "depends_on", From: "lb", To: "db"},
		},
	}
	result, err := ingest.IngestCanvas(doc, providers.Registry(loadRegistry(t)), 1)
	if err != nil {
		t.Fatalf("IngestCanvas: %v", err)
	}

	tr := core.BuildTrace(result.IR, "", "db", "0.0.0.0/0", "tcp", 5432)
	if tr.Allowed {
		t.Fatal("got allowed, want not_assessable — db has no security group attached at all")
	}
	var last core.TraceStep
	for _, s := range tr.Steps {
		last = s
	}
	if last.Step != "sg_dest_ingress" || last.Decision != core.TraceNotAssessable {
		t.Fatalf("got last step %+v, want a not_assessable sg_dest_ingress step", last)
	}
}
