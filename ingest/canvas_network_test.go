package ingest_test

// PC-138/PC-139: canvas-authored routes and NACL rules must produce the same IR shapes —
// and therefore the same engine decisions — as the equivalent Terraform. Both producers
// are ingested for real; every comparison is against the Terraform-built IR, and again
// after the JSON round trip every stored version goes through.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"preflight/core"
	"preflight/ingest"
	"preflight/providers"
)

const networkTerraform = `
resource "aws_vpc" "v" {
  cidr_block = "10.0.0.0/16"
}
resource "aws_route53_record" "entry" {}
resource "aws_db_instance" "db" {}
resource "aws_subnet" "s" {
  vpc_id            = aws_vpc.v.id
  availability_zone = "eu-west-1a"
  cidr_block        = "10.0.0.0/24"
}
resource "aws_internet_gateway" "igw" {
  vpc_id = aws_vpc.v.id
}
resource "aws_route_table" "rt" {
  vpc_id = aws_vpc.v.id
  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.igw.id
  }
}
resource "aws_route_table_association" "a" {
  subnet_id      = aws_subnet.s.id
  route_table_id = aws_route_table.rt.id
}
resource "aws_network_acl" "n" {
  vpc_id = aws_vpc.v.id
  ingress {
    rule_no     = 100
    protocol    = "tcp"
    action      = "allow"
    cidr_block  = "0.0.0.0/0"
    from_port   = 443
    to_port     = 443
  }
}
resource "aws_network_acl_rule" "deny_ssh" {
  network_acl_id = aws_network_acl.n.id
  rule_number    = 90
  egress         = false
  protocol       = "tcp"
  rule_action    = "deny"
  cidr_block     = "198.51.100.0/24"
  from_port      = 22
  to_port        = 22
}
resource "aws_network_acl_association" "a" {
  subnet_id      = aws_subnet.s.id
  network_acl_id = aws_network_acl.n.id
}
`

func networkCanvas() core.CanvasDocument {
	n := func(id, service string, typ core.NodeType) core.CanvasNode {
		return core.CanvasNode{ID: id, Type: typ, Label: id, Capability: map[string]string{}, ServiceID: service}
	}
	vpc := n("aws_vpc.v", "aws_vpc", core.NodeTypeNetworkBoundary)
	vpc.CIDRBlock = "10.0.0.0/16"
	sub := n("aws_subnet.s", "aws_subnet", core.NodeTypeNetworkBoundary)
	sub.CIDRBlock, sub.AvailabilityZone = "10.0.0.0/24", "eu-west-1a"
	rt := n("aws_route_table.rt", "aws_route_table", core.NodeTypeNetworkBoundary)
	rt.Routes = []core.CanvasRoute{{DestinationCIDR: "0.0.0.0/0", Target: "aws_internet_gateway.igw"}}
	nacl := n("aws_network_acl.n", "aws_network_acl", core.NodeTypeNetworkBoundary)
	nacl.NACLRules = []core.CanvasNACLRule{
		{Direction: "ingress", Number: 100, Protocol: "tcp", FromPort: 443, ToPort: 443, CIDRBlock: "0.0.0.0/0", Action: "allow"},
		{Direction: "ingress", Number: 90, Protocol: "tcp", FromPort: 22, ToPort: 22, CIDRBlock: "198.51.100.0/24", Action: "deny"},
	}
	e := func(id string, t core.EdgeType, from, to string) core.CanvasEdge {
		return core.CanvasEdge{ID: id, Type: t, From: from, To: to}
	}
	return core.CanvasDocument{
		Nodes: []core.CanvasNode{vpc, sub, n("aws_internet_gateway.igw", "aws_internet_gateway", core.NodeTypeNetworkBoundary), rt, nacl,
			n("aws_route53_record.entry", "aws_route53_record", core.NodeTypeDNS), n("aws_db_instance.db", "aws_db_instance", core.NodeTypeManagedDatabase)},
		Edges: []core.CanvasEdge{
			e("e1", core.EdgeTypeContainedIn, "aws_subnet.s", "aws_vpc.v"),
			e("e2", core.EdgeTypeContainedIn, "aws_internet_gateway.igw", "aws_vpc.v"),
			e("e3", core.EdgeTypeDependsOn, "aws_subnet.s", "aws_route_table.rt"), // route table association
			e("e4", core.EdgeTypeDependsOn, "aws_subnet.s", "aws_network_acl.n"),  // NACL association
		},
	}
}

func jsonRoundTrip(t *testing.T, ir *core.IR) *core.IR {
	t.Helper()
	b, err := json.Marshal(ir)
	if err != nil {
		t.Fatal(err)
	}
	var out core.IR
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return &out
}

type routeFact struct{ From, To, CIDR, Kind string }

func routeFacts(ir *core.IR) []routeFact {
	var out []routeFact
	for _, e := range ir.Edges {
		if e.Type != core.EdgeTypeRoutesTo {
			continue
		}
		cidr, _ := e.RawAttributes["destination_cidr"].(string)
		kind, _ := e.RawAttributes["target_kind"].(string)
		out = append(out, routeFact{e.From, e.To, cidr, kind})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].From+out[i].To+out[i].CIDR < out[j].From+out[j].To+out[j].CIDR })
	return out
}

// decisions is everything the PC-111/PC-113 engines would answer for this subnet.
func decisions(t *testing.T, ir *core.IR) map[string]any {
	t.Helper()
	isPublic, hasRT := core.IsPublicSubnet(ir.Edges, "aws_subnet.s")
	rtID, rtOK := core.EffectiveRouteTableID(ir.Edges, "aws_subnet.s")
	profile, ok := core.NACLProfileForSubnet(ir.Nodes, ir.Edges, "aws_subnet.s")
	if !ok {
		t.Fatal("no NACL profile resolved for the subnet")
	}
	sort.Slice(profile.Rules, func(i, j int) bool { return profile.Rules[i].Number < profile.Rules[j].Number })
	out := map[string]any{"public": isPublic, "hasRT": hasRT, "rt": rtID, "rtOK": rtOK, "nacl": profile}
	for name, req := range map[string]struct {
		cidr, proto string
		port        int
	}{
		"ssh-from-denied-range": {"198.51.100.7/32", "tcp", 22},
		"https-from-anywhere":   {"203.0.113.9/32", "tcp", 443},
		"http-no-rule":          {"203.0.113.9/32", "tcp", 80},
	} {
		d := core.EvaluateNACLDirectional(profile, "ingress", req.cidr, req.proto, req.port)
		out["decision:"+name] = d
	}
	return out
}

func TestIngestCanvas_RoutesAndNACLs_ParityWithTerraform(t *testing.T) {
	reg := providers.Registry(loadRegistry(t))

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(networkTerraform), 0o644); err != nil {
		t.Fatal(err)
	}
	tf, err := ingest.Ingest(dir, reg, 1)
	if err != nil || tf.IR == nil {
		t.Fatalf("terraform ingest: %v / %+v", err, tf.Insufficient)
	}
	cv, err := ingest.IngestCanvas(networkCanvas(), reg, 1)
	if err != nil || cv.IR == nil {
		t.Fatalf("canvas ingest: %v / %+v", err, cv.Insufficient)
	}

	if len(routeFacts(tf.IR)) == 0 {
		t.Fatal("test setup: the Terraform bundle produced no routes_to edge — the comparison would be vacuous")
	}
	for name, pair := range map[string][2]*core.IR{
		"in memory":         {tf.IR, cv.IR},
		"after JSON reload": {jsonRoundTrip(t, tf.IR), jsonRoundTrip(t, cv.IR)},
	} {
		if a, b := routeFacts(pair[0]), routeFacts(pair[1]); !reflect.DeepEqual(a, b) {
			t.Fatalf("%s: routes_to edges differ\nterraform: %+v\ncanvas:    %+v", name, a, b)
		}
		a, b := decisions(t, pair[0]), decisions(t, pair[1])
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("%s: engine decisions differ\nterraform: %+v\ncanvas:    %+v", name, a, b)
		}
	}

	// Non-vacuous: the decisions are the ones the rules are supposed to produce.
	d := decisions(t, cv.IR)
	if d["public"] != true || d["rt"] != "aws_route_table.rt" {
		t.Errorf("canvas subnet should resolve to its public route table, got %+v", d)
	}
	deny := d["decision:ssh-from-denied-range"].(core.NACLDecision)
	allow := d["decision:https-from-anywhere"].(core.NACLDecision)
	none := d["decision:http-no-rule"].(core.NACLDecision)
	if deny.Allowed || !allow.Allowed || none.Allowed {
		t.Errorf("want ssh denied (rule 90), https allowed (rule 100), http implicitly denied; got %+v %+v %+v", deny, allow, none)
	}
}

func TestIngestCanvas_RouteToUnmodelledTargetIsUnresolvedNeverASilentDrop(t *testing.T) {
	doc := networkCanvas()
	for i := range doc.Nodes {
		if doc.Nodes[i].ID == "aws_route_table.rt" {
			doc.Nodes[i].Routes = []core.CanvasRoute{{DestinationCIDR: "10.9.0.0/16", Target: "aws_vpc.v"}} // a VPC is not a route target kind
		}
	}
	cv, err := ingest.IngestCanvas(doc, providers.Registry(loadRegistry(t)), 1)
	if err != nil || cv.IR == nil {
		t.Fatal(err)
	}
	var found bool
	for _, e := range cv.IR.Edges {
		if e.Type == core.EdgeTypeRoutesTo {
			found = true
			if e.Resolution != core.ResolutionUnresolved {
				t.Errorf("a route to an unmodelled target must be unresolved, got %q", e.Resolution)
			}
			if _, has := e.RawAttributes["target_kind"]; has {
				t.Errorf("an unmodelled target must not be given a target_kind: %+v", e.RawAttributes)
			}
		}
	}
	if !found {
		t.Fatal("the route was silently dropped — I4 forbids that")
	}
}
