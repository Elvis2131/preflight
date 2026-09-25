package ingest_test

// PC-111's own synthetic fixture (testdata/routes-fixture), not the golden bundle:
// golden/aws only exercises aws_route_table's inline route{} blocks with supported
// targets (IGW/NAT). This fixture adds the standalone aws_route resource shape and an
// unsupported route target (transit_gateway_id), so both get real, first-party
// coverage rather than being asserted only by reading the code.

import (
	"path/filepath"
	"testing"

	"preflight/ingest"
)

func TestRouteEdges_BothShapes_AndUnsupportedTarget(t *testing.T) {
	reg := loadRegistry(t)
	result, err := ingest.Ingest(filepath.Join("testdata", "routes-fixture"), reg, 1)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if result.Insufficient != nil {
		t.Fatalf("got insufficient_model: %+v", result.Insufficient)
	}

	foundIGWRoute := false
	for _, e := range result.IR.Edges {
		if e.Type != "routes_to" {
			continue
		}
		if e.From == "aws_route_table.rt" && e.To == "aws_internet_gateway.igw" {
			foundIGWRoute = true
			if e.RawAttributes["destination_cidr"] != "0.0.0.0/0" {
				t.Errorf("igw route destination_cidr = %v, want 0.0.0.0/0", e.RawAttributes["destination_cidr"])
			}
		}
	}
	if !foundIGWRoute {
		t.Error("expected a routes_to edge from the inline route{} block (aws_route_table.rt -> aws_internet_gateway.igw)")
	}

	// The standalone aws_route resource's own target (transit_gateway_id) is
	// explicitly unsupported — it must produce NO routes_to edge (no fabricated
	// success) and must appear in UnsupportedRoutes (never silently dropped, I4).
	for _, e := range result.IR.Edges {
		if e.From == "aws_route_table.rt" && e.RawAttributes["destination_cidr"] == "10.100.0.0/16" {
			t.Errorf("an unsupported-target route produced a real edge: %+v — it must not", e)
		}
	}

	if len(result.UnsupportedRoutes) != 1 {
		t.Fatalf("got %d unsupported routes, want 1", len(result.UnsupportedRoutes))
	}
	got := result.UnsupportedRoutes[0]
	if got.RouteTableKey != "aws_route_table.rt" || got.TargetKind != "transit_gateway" {
		t.Errorf("unsupported route = %+v, want RouteTableKey=aws_route_table.rt, TargetKind=transit_gateway", got)
	}

	// aws_route itself must not appear in OutOfVocabulary (it IS a recognized
	// resource, just one that produces routes rather than a node) and must be
	// tracked as EdgeOnlyResources with Produced=false (an unsupported target
	// produced no edge).
	for _, oov := range result.OutOfVocabulary {
		if oov.ResourceType == "aws_route" {
			t.Error("aws_route must not be reported as out-of-vocabulary")
		}
	}
	foundEdgeOnly := false
	for _, eo := range result.EdgeOnlyResources {
		if eo.ResourceType == "aws_route" && eo.ResourceName == "unsupported" {
			foundEdgeOnly = true
			if eo.Produced {
				t.Error("aws_route.unsupported: Produced = true, want false (its target is unsupported)")
			}
		}
	}
	if !foundEdgeOnly {
		t.Error("expected aws_route.unsupported in EdgeOnlyResources")
	}
}
