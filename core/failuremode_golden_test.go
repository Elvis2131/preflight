package core_test

// This file is PC-17's third acceptance criterion: "not_assessable is produced for at
// least one resilience finding in the golden fixture tests, with a stated reason." It
// builds a real core.FailureMode for the golden bundle's actual NAT-gateway SPOF
// (golden/aws-broken/network.tf's defect 1), wiring together PC-14/PC-78's real
// engines (core.ContainmentBlastRadius, core.RTOFeasibility, core.RPOFeasibility) with
// PC-17's new dimension derivations — not a synthetic stand-in.

import (
	"testing"

	"preflight/core"
	"preflight/ingest"
	awsprovider "preflight/providers/aws"
)

func TestFailureMode_AgainstGoldenBrokenBundle_NATGatewaySPOF(t *testing.T) {
	reg, err := awsprovider.Load()
	if err != nil {
		t.Fatalf("providers/aws.Load(): %v", err)
	}
	workload, err := ingest.LoadWorkload("../golden/workload.yaml")
	if err != nil {
		t.Fatalf("LoadWorkload: %v", err)
	}
	result, err := ingest.Ingest("../golden/aws-broken", reg, 1)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	var containmentEdges []core.DirectedEdge
	for _, e := range result.IR.Edges {
		if e.Type == core.EdgeTypeContainedIn {
			containmentEdges = append(containmentEdges, core.DirectedEdge{From: e.From, To: e.To})
		}
	}

	blastRadius := core.ContainmentBlastRadius(containmentEdges, "aws_subnet.public_a")
	if len(blastRadius) == 0 {
		t.Fatal("expected a real blast radius from the broken bundle's single-NAT topology")
	}

	prov := core.NewProvenance(core.KindDerived, "core:failure-mode-test")

	rdsNode := findNode(t, result.IR.Nodes, "aws_db_instance.payments")
	rm, fm := core.DeriveNodeReplicationAndFailover(rdsNode)

	failureMode := core.FailureMode{
		Trigger:            "AZ eu-west-1a loss",
		AffectedComponents: []string{"aws_nat_gateway.nat_a"},
		BlastRadius:        blastRadius,
		Detection:          core.DetectionModeled,
		ExistingMitigation: "none",
		Gap:                "no second NAT gateway in another AZ",
		Impact:             core.DeriveImpact(len(blastRadius), workload.Criticality, prov).ToEnvelope(),
		Likelihood:         core.DeriveLikelihood(prov).ToEnvelope(),
		Detectability:      core.DeriveDetectability(core.DetectionModeled, prov).ToEnvelope(),
		Recoverability: core.Recoverability{
			FailoverPathExists: core.RTOFeasibility(fm, prov).ToEnvelope(),
			RPOFeasible:        rpoEnvelope(rm, workload, prov),
		},
	}

	if err := failureMode.Validate(); err != nil {
		t.Fatalf("FailureMode failed its own frozen schema validation: %v", err)
	}

	// THE acceptance criterion: Likelihood must be not_assessable, with a real,
	// non-empty reason — not because this test forced it, but because that is what
	// core.DeriveLikelihood always, correctly, returns for structural analysis.
	if failureMode.Likelihood.State != core.AssessmentStateNotAssessable {
		t.Fatalf("Likelihood.State = %q, want not_assessable", failureMode.Likelihood.State)
	}
	if failureMode.Likelihood.Reason == "" {
		t.Fatal("Likelihood must carry a non-empty reason when not_assessable (I4)")
	}

	// Impact and Detectability, by contrast, ARE knowable here — confirming this
	// isn't a test where everything happens to be not_assessable.
	if failureMode.Impact.State != core.AssessmentStateAssessed {
		t.Errorf("Impact.State = %q, want assessed (blast radius and criticality are both known)", failureMode.Impact.State)
	}
	if failureMode.Detectability.State != core.AssessmentStateAssessed {
		t.Errorf("Detectability.State = %q, want assessed (Detection is modeled)", failureMode.Detectability.State)
	}
}

func rpoEnvelope(replicationMode *string, workload core.Workload, prov core.Provenance) core.AssessmentEnvelope {
	rpoTarget, _ := core.RequirementValue(workload, "rpo_seconds")
	return core.RPOFeasibility(&rpoTarget, replicationMode, prov).ToEnvelope()
}
