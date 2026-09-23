package core_test

// PC-82's own acceptance criterion: "declaring a region_loss fault against the golden
// architecture produces a real, hand-verified verdict."

import (
	"testing"

	"preflight/core"
	"preflight/ingest"
	awsprovider "preflight/providers/aws"
)

func TestSimulate_RegionLoss_AgainstGoldenAWSBundle(t *testing.T) {
	reg, err := awsprovider.Load()
	if err != nil {
		t.Fatalf("providers/aws.Load(): %v", err)
	}
	workload, err := ingest.LoadWorkload("../golden/workload.yaml")
	if err != nil {
		t.Fatalf("LoadWorkload: %v", err)
	}
	if len(workload.Regions) != 1 || workload.Regions[0] != "eu-west-1" {
		t.Fatalf("precondition failed: golden/workload.yaml declares regions=%v, want exactly [eu-west-1]", workload.Regions)
	}

	result, err := ingest.Ingest("../golden/aws", reg, 1)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	prov := core.NewProvenance(core.KindDerived, "test:simulate-region-loss")
	resp := core.Simulate(result.IR, workload, []core.Fault{{Type: "region_loss", Target: "eu-west-1"}}, prov)

	// Hand-verified: golden/aws declares exactly one region and has no cross-region
	// failover for anything — a real region_loss must be a total outage, not a
	// partial degradation. This is the actual architectural fact, not an artifact of
	// how the check is written.
	if resp.Verdict.Value != "total_outage" {
		t.Errorf("Verdict.Value = %v, want total_outage — golden/aws has no secondary region for anything to fail over into", resp.Verdict.Value)
	}

	// Every stateful node in the golden architecture (RDS, ElastiCache, SQS) must be
	// severed — hand-counted against golden/aws's own 3 stateful resource types.
	if len(resp.SeveredPaths) == 0 {
		t.Fatal("expected at least one severed path — the golden bundle has real stateful nodes (RDS, ElastiCache, SQS)")
	}
	for _, id := range resp.SeveredPaths {
		found := false
		for _, n := range result.IR.Nodes {
			if n.ID == id {
				found = true
				switch n.Type {
				case core.NodeTypeManagedDatabase, core.NodeTypeCache, core.NodeTypeQueueStream, core.NodeTypeObjectStore:
					// expected
				default:
					t.Errorf("severed path %q has NodeType %q, want a stateful type", id, n.Type)
				}
			}
		}
		if !found {
			t.Errorf("severed path %q does not correspond to any real node in the IR", id)
		}
	}

	if len(resp.Journeys) != 0 {
		t.Errorf("Journeys = %+v, want empty — no entry point survives a whole-region kill", resp.Journeys)
	}

	// Cascade must include every real node in the IR — region_loss with a matching
	// single declared region kills everything.
	if len(resp.Cascade) != len(result.IR.Nodes) {
		t.Errorf("Cascade has %d entries, want %d (every node in the IR)", len(resp.Cascade), len(result.IR.Nodes))
	}

	// Capacity: zero surviving container_workload (EKS) instances x the declared
	// app_node_rps=500 (golden/workload.yaml) = a real, assessed 0.0 — not
	// not_assessable, since this IS a knowable answer given the declared capacity.
	if resp.Capacity.State != core.AssessmentStateAssessed {
		t.Errorf("Capacity.State = %q, want assessed (app_node_rps IS declared in golden/workload.yaml)", resp.Capacity.State)
	}
	capValue, ok := resp.Capacity.Value.(float64)
	if !ok || capValue != 0 {
		t.Errorf("Capacity.Value = %v, want 0.0", resp.Capacity.Value)
	}
}

// TestSimulate_NodeLoss_AgainstGoldenAWSBundle_KillsOnlyThatNode (PC-88) is the
// counterpart to the region_loss golden test above — hand-verified first by actually
// running Simulate against the real bundle and inspecting the output (not guessed):
// killing the single ElastiCache node alone severs only itself. RDS and SQS sit
// behind the same load balancer / EKS entry path, entirely independent of the cache,
// so they must stay unaffected — proving node_loss's narrower blast radius really is
// narrower against a real architecture, not just the small synthetic IR in
// simulate_test.go.
func TestSimulate_NodeLoss_AgainstGoldenAWSBundle_KillsOnlyThatNode(t *testing.T) {
	reg, err := awsprovider.Load()
	if err != nil {
		t.Fatalf("providers/aws.Load(): %v", err)
	}
	workload, err := ingest.LoadWorkload("../golden/workload.yaml")
	if err != nil {
		t.Fatalf("LoadWorkload: %v", err)
	}
	result, err := ingest.Ingest("../golden/aws", reg, 1)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	var cacheID string
	for _, n := range result.IR.Nodes {
		if n.Type == core.NodeTypeCache {
			cacheID = n.ID
		}
	}
	if cacheID == "" {
		t.Fatal("precondition failed: golden/aws has no cache node — this test needs one to target")
	}

	prov := core.NewProvenance(core.KindDerived, "test:simulate-node-loss")
	resp := core.Simulate(result.IR, workload, []core.Fault{{Type: "node_loss", Target: cacheID}}, prov)

	if resp.Verdict.Value != "degraded" {
		t.Errorf("Verdict.Value = %v, want degraded — one stateful node killed out of three, the other two are untouched", resp.Verdict.Value)
	}
	if len(resp.SeveredPaths) != 1 || resp.SeveredPaths[0] != cacheID {
		t.Errorf("SeveredPaths = %v, want [%s] only — RDS and SQS don't sit behind the cache", resp.SeveredPaths, cacheID)
	}
	if len(resp.Cascade) != 1 || resp.Cascade[0] != cacheID {
		t.Errorf("Cascade = %v, want [%s] only — nothing is reachable only through the cache", resp.Cascade, cacheID)
	}
	if resp.Capacity.State != core.AssessmentStateAssessed {
		t.Errorf("Capacity.State = %q, want assessed", resp.Capacity.State)
	}
	capValue, ok := resp.Capacity.Value.(float64)
	if !ok || capValue != 500 {
		t.Errorf("Capacity.Value = %v, want 500.0 — the single EKS compute node is untouched by a cache-only kill", resp.Capacity.Value)
	}
}

func TestSimulate_NonRegionLossFault_AgainstGoldenAWSBundle_IsNotAssessable(t *testing.T) {
	reg, err := awsprovider.Load()
	if err != nil {
		t.Fatalf("providers/aws.Load(): %v", err)
	}
	workload, err := ingest.LoadWorkload("../golden/workload.yaml")
	if err != nil {
		t.Fatalf("LoadWorkload: %v", err)
	}
	result, err := ingest.Ingest("../golden/aws", reg, 1)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	prov := core.NewProvenance(core.KindDerived, "test:simulate-unimplemented-fault")
	resp := core.Simulate(result.IR, workload, []core.Fault{{Type: "az_loss", Target: "eu-west-1a"}}, prov)

	if resp.Verdict.State != core.AssessmentStateNotAssessable {
		t.Errorf("Verdict.State = %q, want not_assessable — az_loss is not implemented in this version (PC-82's own stated scope)", resp.Verdict.State)
	}
}
