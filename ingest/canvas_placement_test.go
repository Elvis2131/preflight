package ingest_test

// PC-105: a canvas-built VPC/subnet architecture must produce the same containment
// structure — and therefore the same zone-loss lost set — as the equivalent Terraform.
// Both producers are ingested for real and compared with reflect.DeepEqual, never
// against hand-copied expectations alone.

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"preflight/core"
	"preflight/ingest"
	"preflight/providers"
)

const placementTerraform = `
resource "aws_vpc" "main" {
  cidr_block = "10.0.0.0/16"
}
resource "aws_subnet" "a" {
  vpc_id            = aws_vpc.main.id
  cidr_block        = "10.0.1.0/24"
  availability_zone = "eu-west-1a"
}
resource "aws_subnet" "b" {
  vpc_id            = aws_vpc.main.id
  cidr_block        = "10.0.2.0/24"
  availability_zone = "eu-west-1b"
}
resource "aws_nat_gateway" "nat_a" {
  subnet_id = aws_subnet.a.id
}
resource "aws_lb" "web" {
  load_balancer_type = "application"
  subnets            = [aws_subnet.a.id, aws_subnet.b.id]
}
resource "aws_db_subnet_group" "g" {
  subnet_ids = [aws_subnet.a.id, aws_subnet.b.id]
}
resource "aws_db_instance" "db" {
  db_subnet_group_name = aws_db_subnet_group.g.name
}
`

func placementCanvas() core.CanvasDocument {
	n := func(id, serviceID string, typ core.NodeType) core.CanvasNode {
		return core.CanvasNode{ID: id, Type: typ, Label: id, Capability: map[string]string{}, ServiceID: serviceID}
	}
	vpc := n("aws_vpc.main", "aws_vpc", core.NodeTypeNetworkBoundary)
	vpc.CIDRBlock = "10.0.0.0/16"
	a := n("aws_subnet.a", "aws_subnet", core.NodeTypeNetworkBoundary)
	a.CIDRBlock, a.AvailabilityZone = "10.0.1.0/24", "eu-west-1a"
	b := n("aws_subnet.b", "aws_subnet", core.NodeTypeNetworkBoundary)
	b.CIDRBlock, b.AvailabilityZone = "10.0.2.0/24", "eu-west-1b"
	lb := n("aws_lb.web", "aws_lb", core.NodeTypeLoadBalancer)
	lb.Sizing = map[string]string{"load_balancer_type": "application"}
	in := func(id, from, to string) core.CanvasEdge {
		return core.CanvasEdge{ID: id, Type: "contained_in", From: from, To: to}
	}
	return core.CanvasDocument{
		Nodes: []core.CanvasNode{vpc, a, b, n("aws_nat_gateway.nat_a", "aws_nat_gateway", core.NodeTypeNetworkBoundary), lb,
			n("aws_db_subnet_group.g", "aws_db_subnet_group", core.NodeTypeNetworkBoundary),
			n("aws_db_instance.db", "aws_db_instance", core.NodeTypeManagedDatabase)},
		Edges: []core.CanvasEdge{
			in("e1", "aws_subnet.a", "aws_vpc.main"), in("e2", "aws_subnet.b", "aws_vpc.main"),
			in("e3", "aws_nat_gateway.nat_a", "aws_subnet.a"),
			in("e4", "aws_lb.web", "aws_subnet.a"), in("e5", "aws_lb.web", "aws_subnet.b"),
			in("e6", "aws_db_subnet_group.g", "aws_subnet.a"), in("e7", "aws_db_subnet_group.g", "aws_subnet.b"),
			in("e8", "aws_db_instance.db", "aws_db_subnet_group.g"),
		},
	}
}

func containment(ir *core.IR) []core.DirectedEdge {
	var out []core.DirectedEdge
	for _, e := range ir.Edges {
		if e.Type == core.EdgeTypeContainedIn {
			out = append(out, core.DirectedEdge{From: e.From, To: e.To})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		return out[i].To < out[j].To
	})
	return out
}

func TestIngestCanvas_PlacementParityWithTerraform_ZoneLossLostSet(t *testing.T) {
	reg := providers.Registry(loadRegistry(t))

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(placementTerraform), 0o644); err != nil {
		t.Fatal(err)
	}
	tf, err := ingest.Ingest(dir, reg, 1)
	if err != nil || tf.IR == nil {
		t.Fatalf("terraform ingest: %v / %+v", err, tf.Insufficient)
	}
	cv, err := ingest.IngestCanvas(placementCanvas(), reg, 1)
	if err != nil || cv.IR == nil {
		t.Fatalf("canvas ingest: %v / %+v", err, cv.Insufficient)
	}

	tfEdges, cvEdges := containment(tf.IR), containment(cv.IR)
	if len(tfEdges) == 0 {
		t.Fatal("test setup: Terraform ingest produced no contained_in edges — the bundle does not exercise containment")
	}
	if !reflect.DeepEqual(tfEdges, cvEdges) {
		t.Fatalf("contained_in edges differ\nterraform: %+v\ncanvas:    %+v", tfEdges, cvEdges)
	}

	// The zone-loss lost set: kill a subnet, take everything contained in it.
	for _, subnet := range []string{"aws_subnet.a", "aws_subnet.b"} {
		tfLost := core.ContainmentBlastRadius(tfEdges, subnet)
		cvLost := core.ContainmentBlastRadius(cvEdges, subnet)
		sort.Strings(tfLost)
		sort.Strings(cvLost)
		if subnet == "aws_subnet.a" && !contains(tfLost, "aws_nat_gateway.nat_a") {
			t.Fatalf("test setup: losing %s must take its NAT gateway with it, got %v — the comparison would be vacuous", subnet, tfLost)
		}
		if !reflect.DeepEqual(tfLost, cvLost) {
			t.Fatalf("zone loss of %s: lost sets differ\nterraform: %v\ncanvas:    %v", subnet, tfLost, cvLost)
		}
	}

	// And the placement attributes themselves carry the same values under the same keys.
	attr := func(ir *core.IR, id, key string) any {
		for _, n := range ir.Nodes {
			if n.ID == id {
				return n.RawAttributes[key]
			}
		}
		return nil
	}
	for _, c := range []struct{ id, key string }{
		{"aws_subnet.a", "availability_zone"}, {"aws_subnet.b", "availability_zone"},
		{"aws_subnet.a", "cidr_block"}, {"aws_vpc.main", "cidr_block"},
	} {
		if tv, cvv := attr(tf.IR, c.id, c.key), attr(cv.IR, c.id, c.key); tv == nil || tv != cvv {
			t.Errorf("%s %s: terraform=%v canvas=%v, want identical non-nil values", c.id, c.key, tv, cvv)
		}
	}

	// The canvas document is valid placement by the server's own rules.
	if vs := core.ValidateCanvasPlacement(placementCanvas()); len(vs) != 0 {
		t.Errorf("the equivalent canvas must be valid placement, got %+v", vs)
	}
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
