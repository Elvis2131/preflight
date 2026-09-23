package core_test

// This file exercises core.RPOFeasibility/RTOFeasibility/DeriveNodeReplicationAndFailover
// (PC-14's fourth acceptance criterion) against the REAL golden fixtures — it needs
// ingest + providers/aws, which is exactly why it lives OUTSIDE core/internal/analyse
// (see core/assess.go's doc comment: core is the only public surface onto core's own
// internal packages; a test proving that surface works correctly from the outside
// belongs on the outside).

import (
	"testing"

	"preflight/core"
	"preflight/ingest"
	awsprovider "preflight/providers/aws"
)

func TestRTORPOFeasibility_AgainstGoldenBundles(t *testing.T) {
	reg, err := awsprovider.Load()
	if err != nil {
		t.Fatalf("providers/aws.Load(): %v", err)
	}
	workload, err := ingest.LoadWorkload("../golden/workload.yaml")
	if err != nil {
		t.Fatalf("LoadWorkload: %v", err)
	}

	t.Run("clean bundle: RDS multi_az=true satisfies RPO=0 and has a real failover path", func(t *testing.T) {
		result, err := ingest.Ingest("../golden/aws", reg, 1)
		if err != nil {
			t.Fatalf("Ingest: %v", err)
		}
		node := findNode(t, result.IR.Nodes, "aws_db_instance.payments")

		rm, fm := core.DeriveNodeReplicationAndFailover(node)
		if rm == nil || *rm != "sync" {
			t.Fatalf("ReplicationMode = %v, want sync (golden/aws sets multi_az = true)", rm)
		}

		rpoTarget, ok := core.RequirementValue(workload, "rpo_seconds")
		if !ok || rpoTarget != 0 {
			t.Fatalf("expected rpo_seconds=0 declared in workload.yaml, got %v (%v)", rpoTarget, ok)
		}
		prov := core.NewProvenance(core.KindDerived, "core:rto-rpo-feasibility-test")

		rpoResult := core.RPOFeasibility(&rpoTarget, rm, prov)
		value, assessed := rpoResult.Value()
		if !assessed || !value {
			t.Fatalf("RPOFeasibility = %+v, want Assessed(true) — sync replication satisfies RPO=0", rpoResult)
		}

		rtoResult := core.RTOFeasibility(fm, prov)
		rtoValue, rtoAssessed := rtoResult.Value()
		if !rtoAssessed || !rtoValue {
			t.Fatalf("RTOFeasibility = %+v, want Assessed(true) — Multi-AZ provides a real failover path", rtoResult)
		}
	})

	t.Run("broken bundle: RDS multi_az=false fails RPO=0 and has no failover path", func(t *testing.T) {
		result, err := ingest.Ingest("../golden/aws-broken", reg, 1)
		if err != nil {
			t.Fatalf("Ingest: %v", err)
		}
		node := findNode(t, result.IR.Nodes, "aws_db_instance.payments")

		rm, fm := core.DeriveNodeReplicationAndFailover(node)
		if rm == nil || *rm != "none" {
			t.Fatalf("ReplicationMode = %v, want none (golden/aws-broken's defect 2 sets multi_az = false)", rm)
		}

		rpoTarget, _ := core.RequirementValue(workload, "rpo_seconds")
		prov := core.NewProvenance(core.KindDerived, "core:rto-rpo-feasibility-test")

		rpoResult := core.RPOFeasibility(&rpoTarget, rm, prov)
		if rpoResult.IsAssessed() {
			t.Fatalf("RPOFeasibility = %+v, want not_assessable — replication mode 'none' has no RPO feasibility rule (it's neither 'sync' nor 'async')", rpoResult)
		}

		rtoResult := core.RTOFeasibility(fm, prov)
		rtoValue, rtoAssessed := rtoResult.Value()
		if !rtoAssessed || rtoValue {
			t.Fatalf("RTOFeasibility = %+v, want Assessed(false) — no standby exists, defect 2 is a real RTO regression", rtoResult)
		}
	})
}

func findNode(t *testing.T, nodes []core.Node, id string) core.Node {
	t.Helper()
	for _, n := range nodes {
		if n.ID == id {
			return n
		}
	}
	t.Fatalf("node %q not found", id)
	return core.Node{}
}
