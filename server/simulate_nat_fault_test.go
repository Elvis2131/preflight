package server_test

// PC-129: /simulate's own new fault types, end-to-end against the real golden bundle.

import (
	"path/filepath"
	"testing"

	"preflight/core"
	"preflight/server"
)

func TestSimulate_NATGatewayLoss_RealFixture(t *testing.T) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	bundleDir, err := filepath.Abs("../golden/aws")
	if err != nil {
		t.Fatal(err)
	}
	workloadPath, err := filepath.Abs("../golden/workload.yaml")
	if err != nil {
		t.Fatal(err)
	}
	assessResp, err := server.Assess(store, server.AssessRequest{SessionID: "nat-fault-test", BundleDir: bundleDir, WorkloadPath: workloadPath})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}

	resp, err := server.Simulate(store, server.SimulateRequest{
		SessionID:     "nat-fault-test",
		VersionNumber: assessResp.VersionNumber,
		Faults:        []core.Fault{{Type: "nat_gateway_loss", Target: "aws_nat_gateway.nat_a"}},
	})
	if err != nil {
		t.Fatalf("Simulate: %v", err)
	}
	found := false
	for _, id := range resp.Cascade {
		if id == "aws_nat_gateway.nat_a" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected aws_nat_gateway.nat_a in the cascade set, got %v", resp.Cascade)
	}
}

func TestSimulate_RouteRemoval_RealFixture(t *testing.T) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	bundleDir, err := filepath.Abs("../golden/aws")
	if err != nil {
		t.Fatal(err)
	}
	workloadPath, err := filepath.Abs("../golden/workload.yaml")
	if err != nil {
		t.Fatal(err)
	}
	assessResp, err := server.Assess(store, server.AssessRequest{SessionID: "route-removal-test", BundleDir: bundleDir, WorkloadPath: workloadPath})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}

	resp, err := server.Simulate(store, server.SimulateRequest{
		SessionID:     "route-removal-test",
		VersionNumber: assessResp.VersionNumber,
		Faults:        []core.Fault{{Type: "route_removal", Target: "aws_route_table.private_a", DestinationCIDR: "0.0.0.0/0"}},
	})
	if err != nil {
		t.Fatalf("Simulate: %v", err)
	}
	if resp.Verdict.State != core.AssessmentStateAssessed && resp.Verdict.State != core.AssessmentStateNotAssessable {
		t.Errorf("Verdict.State = %v, want a real computed state, not an error", resp.Verdict.State)
	}
}

func TestSimulate_RouteRemoval_MissingDestinationCIDR_NotAssessable(t *testing.T) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	bundleDir, err := filepath.Abs("../golden/aws")
	if err != nil {
		t.Fatal(err)
	}
	workloadPath, err := filepath.Abs("../golden/workload.yaml")
	if err != nil {
		t.Fatal(err)
	}
	assessResp, err := server.Assess(store, server.AssessRequest{SessionID: "route-removal-missing-cidr", BundleDir: bundleDir, WorkloadPath: workloadPath})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}

	resp, err := server.Simulate(store, server.SimulateRequest{
		SessionID:     "route-removal-missing-cidr",
		VersionNumber: assessResp.VersionNumber,
		Faults:        []core.Fault{{Type: "route_removal", Target: "aws_route_table.private_a"}},
	})
	if err != nil {
		t.Fatalf("Simulate: %v", err)
	}
	if resp.Verdict.State != core.AssessmentStateNotAssessable {
		t.Errorf("Verdict.State = %v, want not_assessable (no destination_cidr declared)", resp.Verdict.State)
	}
}
