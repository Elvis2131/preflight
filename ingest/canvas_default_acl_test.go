package ingest_test

// PC-153: a canvas-authored default network ACL resolves exactly as Terraform's
// aws_default_network_acl does (PC-149): the DECLARED default for a subnet with no explicit
// association, not the assumed one. The marker is the node's service_id (data: the mapping's
// default_nacl flag), tied to its VPC by a contained_in edge.

import (
	"path/filepath"
	"reflect"
	"testing"

	"preflight/core"
	"preflight/ingest"
	"preflight/providers"
)

func canvasDefaultNACLDoc(naclService string, rules []core.CanvasNACLRule, tieToVPC bool) core.CanvasDocument {
	node := func(id, typ, service string) core.CanvasNode {
		return core.CanvasNode{ID: id, Type: core.NodeType(typ), Label: id, Capability: map[string]string{}, ServiceID: service}
	}
	sub := node("s", "network_boundary", "aws_subnet")
	sub.AvailabilityZone = "eu-west-1a"
	sub.CIDRBlock = "10.0.0.0/24"
	vpc := node("v", "network_boundary", "aws_vpc")
	vpc.CIDRBlock = "10.0.0.0/16"
	nacl := node("d", "network_boundary", naclService)
	nacl.NACLRules = rules
	doc := core.CanvasDocument{
		Nodes: []core.CanvasNode{vpc, sub, nacl, node("entry", "dns", "aws_route53_record"), node("db", "managed_database", "aws_db_instance")},
		Edges: []core.CanvasEdge{{ID: "e1", Type: "contained_in", From: "s", To: "v"}},
	}
	if tieToVPC {
		doc.Edges = append(doc.Edges, core.CanvasEdge{ID: "e2", Type: "contained_in", From: "d", To: "v"})
	}
	return doc
}

var allow443 = []core.CanvasNACLRule{{Direction: "ingress", Number: 100, Protocol: "tcp", FromPort: 443, ToPort: 443, CIDRBlock: "0.0.0.0/0", Action: "allow"}}

func resolveCanvasSubnet(t *testing.T, doc core.CanvasDocument) (core.NACLResolution, bool) {
	t.Helper()
	res, err := ingest.IngestCanvas(doc, providers.Registry(loadRegistry(t)), 1)
	if err != nil || res.IR == nil {
		t.Fatalf("IngestCanvas: %v %+v", err, res.Insufficient)
	}
	return core.ResolveSubnetNACL(res.IR.Nodes, res.IR.Edges, "s")
}

func TestCanvasDefaultNACL_ResolvesAsDeclaredDefault_MatchingTerraform(t *testing.T) {
	canvas, ok := resolveCanvasSubnet(t, canvasDefaultNACLDoc("aws_default_network_acl", allow443, true))
	if !ok || canvas.Source != core.NACLDeclaredDefault || canvas.Profile.NACLID != "d" {
		t.Fatalf("canvas: got %+v ok=%v, want the declared default NACL", canvas, ok)
	}

	tf, err := ingest.Ingest(filepath.Join("testdata", "default-nacl-fixture"), loadRegistry(t), 1)
	if err != nil || tf.IR == nil {
		t.Fatalf("Ingest: %v", err)
	}
	hcl, ok := core.ResolveSubnetNACL(tf.IR.Nodes, tf.IR.Edges, "aws_subnet.s")
	if !ok || hcl.Source != canvas.Source {
		t.Fatalf("hcl: got %+v ok=%v", hcl, ok)
	}
	if !reflect.DeepEqual(canvas.Profile.Rules, hcl.Profile.Rules) {
		t.Errorf("the two producers resolve different rules:\ncanvas %+v\nhcl    %+v", canvas.Profile.Rules, hcl.Profile.Rules)
	}
}

// The marker is what makes it the default: the same rules on an ordinary NACL node that no subnet
// is associated with are not consulted, and a default NACL with no rules authored leaves AWS's
// documented default assumed, tagged so.
func TestCanvasDefaultNACL_NegativeControls(t *testing.T) {
	ordinary, ok := resolveCanvasSubnet(t, canvasDefaultNACLDoc("aws_network_acl", allow443, true))
	if !ok || ordinary.Source != core.NACLAssumedDefault {
		t.Errorf("an ordinary NACL with no association must not act as the default: got %+v ok=%v", ordinary, ok)
	}
	empty, ok := resolveCanvasSubnet(t, canvasDefaultNACLDoc("aws_default_network_acl", nil, true))
	if !ok || empty.Source != core.NACLAssumedDefault {
		t.Errorf("a default NACL with no rules authored must leave the default assumed: got %+v ok=%v", empty, ok)
	}
}
