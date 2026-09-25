package core_test

// PC-126's own acceptance criterion, verbatim: "Bottleneck findings appear in the
// delta when a design change fixes or introduces one — tested on a two-version
// fixture." Reuses the pre-existing compliance delta engine (core.ComputeDelta) —
// no second delta mechanism, since BuildLoadFindings uses the same
// satisfied/unsatisfied vocabulary every other compliance-shaped finding does.

import (
	"testing"

	"preflight/core"
)

func TestBottleneckFindings_FlowIntoDelta_Introduced(t *testing.T) {
	prov := core.NewProvenance(core.KindDerived, "test")

	// Version 1: db is within capacity.
	v1Loads := []core.ComponentLoad{{NodeID: "db", OfferedRPS: 50, Capacity: floatPtr(100), Utilization: floatPtr(0.5)}}
	v1Findings := core.BuildLoadFindings(v1Loads, prov)
	v1Scorecard := core.BuildScorecard(v1Findings, 1)

	// Version 2: a design change doubled offered load (e.g. a new journey added) —
	// db is now over capacity, a real, newly-introduced bottleneck.
	v2Loads := []core.ComponentLoad{{NodeID: "db", OfferedRPS: 150, Capacity: floatPtr(100), Utilization: floatPtr(1.5)}}
	v2Findings := core.BuildLoadFindings(v2Loads, prov)
	v2Scorecard := core.BuildScorecard(v2Findings, 2)

	delta := core.ComputeDelta(v1Scorecard.StatusMap(), v2Scorecard.StatusMap(), prov)
	var found *core.DeltaEntry
	for i := range delta {
		if delta[i].FindingID == "finding.load.bottleneck.db" {
			found = &delta[i]
		}
	}
	if found == nil {
		t.Fatalf("expected finding.load.bottleneck.db in the delta, got %+v", delta)
	}
	if found.Kind != core.DeltaRegression {
		t.Errorf("Kind = %q, want regression (satisfied -> unsatisfied is a newly-introduced bottleneck)", found.Kind)
	}
}

func TestBottleneckFindings_FlowIntoDelta_Fixed(t *testing.T) {
	prov := core.NewProvenance(core.KindDerived, "test")

	v1Loads := []core.ComponentLoad{{NodeID: "db", OfferedRPS: 150, Capacity: floatPtr(100), Utilization: floatPtr(1.5)}}
	v1Findings := core.BuildLoadFindings(v1Loads, prov)
	v1Scorecard := core.BuildScorecard(v1Findings, 1)

	// Version 2: capacity was declared higher (e.g. the architect scaled the
	// component) — the same offered load no longer exceeds it.
	v2Loads := []core.ComponentLoad{{NodeID: "db", OfferedRPS: 150, Capacity: floatPtr(300), Utilization: floatPtr(0.5)}}
	v2Findings := core.BuildLoadFindings(v2Loads, prov)
	v2Scorecard := core.BuildScorecard(v2Findings, 2)

	delta := core.ComputeDelta(v1Scorecard.StatusMap(), v2Scorecard.StatusMap(), prov)
	var found *core.DeltaEntry
	for i := range delta {
		if delta[i].FindingID == "finding.load.bottleneck.db" {
			found = &delta[i]
		}
	}
	if found == nil {
		t.Fatalf("expected finding.load.bottleneck.db in the delta, got %+v", delta)
	}
	if found.Kind != core.DeltaImprovement {
		t.Errorf("Kind = %q, want improvement (unsatisfied -> satisfied is a fixed bottleneck)", found.Kind)
	}
}
