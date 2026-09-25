package core_test

// PC-111's own acceptance criterion, verbatim: "'Public subnet' derived from routes; a
// subnet labelled public with no IGW route is reported as a finding, not trusted."
// golden/aws never exercises the MISMATCH case (every golden subnet is correctly
// labelled), so this is hand-built, synthetic coverage of exactly that case — the same
// "verified in isolation before touching real data" discipline PC-14 already
// established for core/internal/analyse.

import (
	"testing"

	"preflight/core"
)

func routeTableEdge(from, to, cidr, targetKind string) core.Edge {
	return core.Edge{
		ID: from + "->" + to, Type: core.EdgeTypeRoutesTo, From: from, To: to,
		Resolution: core.ResolutionKnown, Provenance: core.NewProvenance(core.KindStated, "test"),
		RawAttributes: map[string]any{"destination_cidr": cidr, "target_kind": targetKind},
	}
}

func associationEdge(subnetID, tableID string) core.Edge {
	return core.Edge{
		ID: subnetID + "->" + tableID, Type: core.EdgeTypeDependsOn, From: subnetID, To: tableID,
		Resolution: core.ResolutionKnown, Provenance: core.NewProvenance(core.KindStated, "test"),
	}
}

func TestIsPublicSubnet_CorrectlyLabelled(t *testing.T) {
	edges := []core.Edge{
		associationEdge("subnet.a", "rt.public"),
		routeTableEdge("rt.public", "igw.x", "0.0.0.0/0", "internet_gateway"),
	}
	isPublic, hasRT := core.IsPublicSubnet(edges, "subnet.a")
	if !hasRT || !isPublic {
		t.Fatalf("got isPublic=%v hasRouteTable=%v, want true/true", isPublic, hasRT)
	}
}

func TestIsPublicSubnet_MismatchLabelledPublicButRoutesToNAT(t *testing.T) {
	edges := []core.Edge{
		associationEdge("subnet.a", "rt.private"),
		routeTableEdge("rt.private", "nat.x", "0.0.0.0/0", "nat_gateway"),
	}
	isPublic, hasRT := core.IsPublicSubnet(edges, "subnet.a")
	if !hasRT || isPublic {
		t.Fatalf("got isPublic=%v hasRouteTable=%v, want false/true (a NAT default route is not a public-subnet route)", isPublic, hasRT)
	}
}

func TestIsPublicSubnet_NoRouteTable_HonestlyUnknown(t *testing.T) {
	isPublic, hasRT := core.IsPublicSubnet(nil, "subnet.a")
	if hasRT || isPublic {
		t.Fatalf("got isPublic=%v hasRouteTable=%v, want false/false (no association at all — unknown, not assumed private)", isPublic, hasRT)
	}
}

// TestBuildFindings_PublicSubnetLabelMismatch_IsReportedAsAFinding is PC-111's own
// acceptance criterion made concrete against the real BuildFindings entry point (not
// just the underlying IsPublicSubnet helper): a subnet tagged Tier=public whose real
// route table has no route to an internet gateway must produce a real, assessed
// finding stating the mismatch — never silently trusted, never dropped.
func TestBuildFindings_PublicSubnetLabelMismatch_IsReportedAsAFinding(t *testing.T) {
	ir := &core.IR{
		SchemaVersion: "1.1.0", VersionNumber: 1, VersionHash: "sha256:test",
		Nodes: []core.Node{
			{
				ID: "subnet.mislabelled", Type: core.NodeTypeNetworkBoundary, Resolution: core.ResolutionKnown,
				RawAttributes: map[string]any{"tags": map[string]any{"Tier": "public"}},
				Provenance:    core.NewProvenance(core.KindStated, "test:1:subnet.mislabelled"),
			},
			{
				ID: "rt.actually-private", Type: core.NodeTypeNetworkBoundary, Resolution: core.ResolutionKnown,
				Provenance: core.NewProvenance(core.KindStated, "test:2:rt.actually-private"),
			},
			{
				ID: "nat.x", Type: core.NodeTypeNetworkBoundary, Resolution: core.ResolutionKnown,
				Provenance: core.NewProvenance(core.KindStated, "test:3:nat.x"),
			},
		},
		Edges: []core.Edge{
			associationEdge("subnet.mislabelled", "rt.actually-private"),
			routeTableEdge("rt.actually-private", "nat.x", "0.0.0.0/0", "nat_gateway"),
		},
	}

	findings := core.BuildFindings(ir, core.Workload{})

	var found *core.Finding
	for i := range findings {
		if findings[i].ID == "finding.routing.public-subnet-label.subnet.mislabelled" {
			found = &findings[i]
		}
	}
	if found == nil {
		t.Fatal("expected a finding.routing.public-subnet-label.* finding for the mislabelled subnet")
	}
	if found.Outcome.State != core.AssessmentStateAssessed {
		t.Fatalf("Outcome.State = %q, want assessed — this is a real, derivable mismatch, not an unknown", found.Outcome.State)
	}
	value, _ := found.Outcome.Value.(string)
	if value == "" || value[:8] != "mismatch" {
		t.Fatalf("Outcome.Value = %q, want it to start with \"mismatch\" — the label must not be silently trusted", value)
	}
	if err := found.Validate(); err != nil {
		t.Fatalf("finding failed schema validation: %v", err)
	}
}
