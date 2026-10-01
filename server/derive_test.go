package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"preflight/core"
	"preflight/golden/templates"
	"preflight/server"
)

// PC-105's derived facts on the REAL 3-tier template: its public subnets (route table with a
// route to the internet gateway) are public, its private and data subnets (route to a NAT
// gateway) are private, each carries its AZ and the region derived from it — and the SAME
// request on a half-drawn canvas (no entry point, no database: below the MVG) still works,
// which the assessment endpoint would refuse.
func TestDeriveCanvas_ThreeTierTemplate_PublicPrivateAndRegionFromRoutesAndAZ(t *testing.T) {
	tpl, err := templates.Load("three-tier-vpc")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := server.DeriveCanvas(server.DeriveRequest{Canvas: tpl.Canvas})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]struct {
		vis core.SubnetVisibility
		az  string
	}{
		"aws_subnet.public_a": {core.SubnetPublic, "eu-west-1a"}, "aws_subnet.public_b": {core.SubnetPublic, "eu-west-1b"},
		"aws_subnet.private_a": {core.SubnetPrivate, "eu-west-1a"}, "aws_subnet.private_b": {core.SubnetPrivate, "eu-west-1b"},
		"aws_subnet.data_a": {core.SubnetPrivate, "eu-west-1a"}, "aws_subnet.data_b": {core.SubnetPrivate, "eu-west-1b"},
	}
	if len(resp.Subnets) != len(want) {
		t.Fatalf("got %d subnet facts, want %d: %+v", len(resp.Subnets), len(want), resp.Subnets)
	}
	for _, f := range resp.Subnets {
		w, ok := want[f.NodeID]
		if !ok || f.Visibility != w.vis || f.AvailabilityZone != w.az || f.Region != "eu-west-1" {
			t.Errorf("%s = %+v, want %s in %s / eu-west-1", f.NodeID, f, w.vis, w.az)
		}
	}
}

func TestDeriveCanvas_RemovingTheRoutesMakesVisibilityNotAssessable_NeverPrivate(t *testing.T) {
	tpl, _ := templates.Load("three-tier-vpc")
	for i := range tpl.Canvas.Nodes {
		if tpl.Canvas.Nodes[i].ID == "aws_route_table.public" {
			tpl.Canvas.Nodes[i].Routes = nil
		}
	}
	resp, err := server.DeriveCanvas(server.DeriveRequest{Canvas: tpl.Canvas})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range resp.Subnets {
		if f.NodeID == "aws_subnet.public_a" && (f.Visibility != core.SubnetVisibilityUnknown || f.Reason == "") {
			t.Fatalf("a subnet whose route table lost its routes has no resolvable route table: want not_assessable, got %+v", f)
		}
	}
}

func TestDeriveCanvasHandler_WorksOnAHalfDrawnCanvas_AndStoresNothing(t *testing.T) {
	half := core.CanvasDocument{Nodes: []core.CanvasNode{
		{ID: "vpc", Type: "network_boundary", Label: "VPC", Capability: map[string]string{}, ServiceID: "aws_vpc", CIDRBlock: "10.0.0.0/16"},
		{ID: "s", Type: "network_boundary", Label: "S", Capability: map[string]string{}, ServiceID: "aws_subnet", AvailabilityZone: "us-east-1a", CIDRBlock: "10.0.1.0/24"},
	}, Edges: []core.CanvasEdge{{ID: "e", Type: "contained_in", From: "s", To: "vpc"}}}
	body, _ := json.Marshal(server.DeriveRequest{Canvas: half})
	rec := httptest.NewRecorder()
	server.DeriveCanvasHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/canvas/derive", strings.NewReader(string(body))))
	if rec.Code != http.StatusOK {
		t.Fatalf("a half-drawn canvas must still derive: %d %s", rec.Code, rec.Body.String())
	}
	var out server.DeriveResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || len(out.Subnets) != 1 || out.Subnets[0].Region != "us-east-1" {
		t.Fatalf("got %+v err %v, want one subnet in us-east-1", out, err)
	}

	bad := httptest.NewRecorder()
	server.DeriveCanvasHandler().ServeHTTP(bad, httptest.NewRequest(http.MethodPost, "/canvas/derive", strings.NewReader("{not json")))
	if bad.Code != http.StatusBadRequest {
		t.Errorf("malformed body: %d, want 400", bad.Code)
	}
}
