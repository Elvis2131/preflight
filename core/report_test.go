package core_test

// PC-120's own acceptance criteria, each cited below.

import (
	"testing"

	"preflight/core"
)

func TestBuildReport_NFRConformance_RPOWiredOthersNotEvaluated(t *testing.T) {
	ir := &core.IR{}
	workload := core.Workload{
		Requirements: []core.Requirement{
			{ID: "availability.target", Value: 99.95, Priority: core.PriorityHard},
			{ID: "rto_seconds", Value: 60.0, Priority: core.PriorityHard},
			{ID: "rpo_seconds", Value: 0.0, Priority: core.PriorityHard},
		},
	}
	prov := syntheticProv()
	findings := []core.Finding{{
		ID: "finding.compliance.rds-rpo-feasibility.db1", Title: "t",
		Dimensions: core.FailureMode{
			Trigger: "t", AffectedComponents: []string{"db1"}, Detection: core.DetectionModeled,
			Impact: core.NotAssessable[any]("x", prov).ToEnvelope(), Likelihood: core.DeriveLikelihood(prov).ToEnvelope(),
			Detectability: core.DeriveDetectability(core.DetectionModeled, prov).ToEnvelope(),
			Recoverability: core.Recoverability{
				FailoverPathExists: core.NotAssessable[any]("x", prov).ToEnvelope(),
				RPOFeasible:        core.Assessed[any]("feasible", prov).ToEnvelope(),
			},
		},
		Evidence: []core.EvidenceRef{{Description: "x"}},
		Outcome:  core.Assessed[any]("satisfied", prov).ToEnvelope(),
	}}
	scorecard := core.BuildScorecard(findings, 1)

	report := core.BuildReport("s1", 1, "<svg>test</svg>", ir, workload, findings, scorecard, nil, core.PriceTable{}, nil)

	byReq := map[string]core.ReportNFREntry{}
	for _, e := range report.NFRConformance {
		byReq[e.RequirementID] = e
	}
	if !byReq["rpo_seconds"].Evaluated {
		t.Errorf("rpo_seconds.Evaluated = false, want true")
	}
	if len(byReq["rpo_seconds"].FindingIDs) != 1 || byReq["rpo_seconds"].FindingIDs[0] != "finding.compliance.rds-rpo-feasibility.db1" {
		t.Errorf("rpo_seconds.FindingIDs = %v, want the one addressing finding", byReq["rpo_seconds"].FindingIDs)
	}
	if byReq["rto_seconds"].Evaluated {
		t.Error("rto_seconds.Evaluated = true, want false — no engine is wired to this requirement today")
	}
	if byReq["rto_seconds"].Reason == "" {
		t.Error("rto_seconds.Reason is empty, want a real reason explaining why it is not evaluated")
	}
	if byReq["availability.target"].Evaluated {
		t.Error("availability.target.Evaluated = true, want false")
	}
	if report.ExecutiveSummary.NFREvaluatedCount != 1 || report.ExecutiveSummary.NFRNotEvaluatedCount != 2 {
		t.Errorf("NFR counts = %d evaluated / %d not, want 1/2", report.ExecutiveSummary.NFREvaluatedCount, report.ExecutiveSummary.NFRNotEvaluatedCount)
	}
}

func TestBuildReport_CostUnavailable_ExplicitReason(t *testing.T) {
	ir := &core.IR{}
	report := core.BuildReport("s1", 1, "<svg>test</svg>", ir, core.Workload{}, nil, core.Scorecard{VersionNumber: 1}, nil, core.PriceTable{}, nil)
	if report.Cost.Available {
		t.Fatal("Cost.Available = true, want false — no CostReport was supplied")
	}
	if report.Cost.UnavailableReason == "" {
		t.Error("Cost.UnavailableReason is empty, want a real reason")
	}
	if report.Cost.Disclaimer == "" {
		t.Error("Cost.Disclaimer is empty, want the standard cost disclaimer text")
	}
}

func TestBuildReport_TrafficUnavailable_NoJourneysDeclared(t *testing.T) {
	ir := &core.IR{}
	report := core.BuildReport("s1", 1, "<svg>test</svg>", ir, core.Workload{}, nil, core.Scorecard{VersionNumber: 1}, nil, core.PriceTable{}, nil)
	if report.Traffic.Available {
		t.Fatal("Traffic.Available = true, want false — no journeys declared")
	}
	if report.Traffic.UnavailableReason == "" {
		t.Error("Traffic.UnavailableReason is empty, want a real reason")
	}
}

func TestBuildReport_NoNullSlices(t *testing.T) {
	ir := &core.IR{}
	report := core.BuildReport("s1", 1, "<svg>test</svg>", ir, core.Workload{}, nil, core.Scorecard{VersionNumber: 1}, nil, core.PriceTable{}, nil)
	if report.Delta == nil {
		t.Error("Delta is nil, want an empty (never null) slice")
	}
	if report.Assumptions == nil {
		t.Error("Assumptions is nil, want an empty (never null) slice")
	}
	if report.Inventory == nil {
		t.Error("Inventory is nil, want an empty (never null) slice")
	}
	if report.FailureModes.Findings == nil {
		t.Error("FailureModes.Findings is nil, want an empty (never null) slice")
	}
}

// TestBuildReport_NoOrphanVerdicts is the Card's own acceptance criterion, verbatim:
// "Every verdict in the report traces to an existing finding/failure-mode/cost record
// by ID — test walks the report and asserts no orphan verdicts." Every compliance
// control result carries a real, non-empty ControlID, every NFR entry's FindingIDs
// name a real Finding.ID actually present in FailureModes.Findings, and the executive
// summary's own counts are DERIVED from (never independent of) those same lists.
func TestBuildReport_NoOrphanVerdicts(t *testing.T) {
	ir := realGoldenIR(t)
	workload := loadGoldenWorkload(t)
	findings := core.BuildFindings(ir, workload)
	scorecard := core.BuildScorecard(findings, 1)
	report := core.BuildReport("s1", 1, "<svg>test</svg>", ir, workload, findings, scorecard, nil, core.PriceTable{}, nil)

	findingIDs := map[string]bool{}
	for _, f := range report.FailureModes.Findings {
		if f.ID == "" {
			t.Error("a finding has an empty ID")
		}
		findingIDs[f.ID] = true
	}

	for _, section := range report.Compliance {
		total := 0
		for _, c := range section.Controls {
			if c.ControlID == "" {
				t.Errorf("%s: a compliance control result has an empty ControlID", section.Framework)
			}
			total++
		}
		summed := section.ResultCounts.Satisfied + section.ResultCounts.Applicable + section.ResultCounts.Partial + section.ResultCounts.Unsatisfied + section.ResultCounts.NotAssessable
		if summed != total {
			t.Errorf("%s: ResultCounts sum to %d, but %d control results exist — executive summary must be derived from the actual list, never a separately-fabricated number", section.Framework, summed, total)
		}
	}

	for _, e := range report.NFRConformance {
		for _, fid := range e.FindingIDs {
			if !findingIDs[fid] {
				t.Errorf("NFR requirement %s cites finding ID %q, which does not exist in FailureModes.Findings — orphan verdict", e.RequirementID, fid)
			}
		}
	}
}

// TestGatherReportAssumptions_FindsAssumedAndStated_AtVariousNestingDepths directly
// exercises the reflection walker with synthetic data — golden/aws's own real report
// today legitimately has ZERO entries (see ReportAssumption's own doc comment: Kind
// assumed is never actually produced anywhere in this codebase, a real, pre-existing,
// already-documented gap), so this proves the MECHANISM works correctly rather than
// relying only on that vacuous real-world case.
func TestGatherReportAssumptions_FindsAssumedAndStated_AtVariousNestingDepths(t *testing.T) {
	ir := &core.IR{}
	assumedProv := core.NewProvenance(core.KindAssumed, "workload.yaml:some_default")
	assumedProv.Reason = "no value declared; using the documented provider default"
	statedProv := core.NewProvenance(core.KindStated, "workload.yaml:region")

	findings := []core.Finding{{
		ID: "finding.x", Title: "t",
		Dimensions: core.FailureMode{
			Trigger: "t", AffectedComponents: []string{"n1"}, Detection: core.DetectionModeled,
			Impact:        core.NotAssessable[any]("x", statedProv).ToEnvelope(), // stated, nested inside a slice element's struct field
			Likelihood:    core.DeriveLikelihood(assumedProv).ToEnvelope(),       // assumed, nested inside a pointer-free Assessment->envelope conversion
			Detectability: core.DeriveDetectability(core.DetectionModeled, syntheticProv()).ToEnvelope(),
			Recoverability: core.Recoverability{
				FailoverPathExists: core.NotAssessable[any]("x", syntheticProv()).ToEnvelope(),
				RPOFeasible:        core.NotAssessable[any]("x", syntheticProv()).ToEnvelope(),
			},
		},
		Evidence: []core.EvidenceRef{{Description: "x"}},
		Outcome:  core.Assessed[any]("satisfied", syntheticProv()).ToEnvelope(),
	}}
	scorecard := core.BuildScorecard(findings, 1)

	report := core.BuildReport("s1", 1, "<svg>test</svg>", ir, core.Workload{}, findings, scorecard, nil, core.PriceTable{}, nil)

	var foundAssumed, foundStated bool
	for _, a := range report.Assumptions {
		if a.Kind == core.KindAssumed && a.Source == "workload.yaml:some_default" {
			foundAssumed = true
			if a.Reason == "" {
				t.Error("assumed entry has no Reason, want the one attached to its Provenance")
			}
		}
		if a.Kind == core.KindStated && a.Source == "workload.yaml:region" {
			foundStated = true
		}
	}
	if !foundAssumed {
		t.Errorf("expected an assumed entry for workload.yaml:some_default, got %+v", report.Assumptions)
	}
	if !foundStated {
		t.Errorf("expected a stated entry for workload.yaml:region, got %+v", report.Assumptions)
	}
}

func TestGatherReportAssumptions_Deduplicates(t *testing.T) {
	ir := &core.IR{}
	statedProv := core.NewProvenance(core.KindStated, "workload.yaml:region")
	findings := []core.Finding{{
		ID: "finding.a", Title: "t",
		Dimensions: core.FailureMode{
			Trigger: "t", AffectedComponents: []string{"n1"}, Detection: core.DetectionModeled,
			Impact: core.NotAssessable[any]("x", statedProv).ToEnvelope(), Likelihood: core.NotAssessable[any]("x", statedProv).ToEnvelope(),
			Detectability: core.NotAssessable[any]("x", statedProv).ToEnvelope(),
			Recoverability: core.Recoverability{
				FailoverPathExists: core.NotAssessable[any]("x", statedProv).ToEnvelope(),
				RPOFeasible:        core.NotAssessable[any]("x", statedProv).ToEnvelope(),
			},
		},
		Evidence: []core.EvidenceRef{{Description: "x"}},
		Outcome:  core.NotAssessable[any]("x", statedProv).ToEnvelope(),
	}}
	scorecard := core.BuildScorecard(findings, 1)
	report := core.BuildReport("s1", 1, "<svg>test</svg>", ir, core.Workload{}, findings, scorecard, nil, core.PriceTable{}, nil)

	count := 0
	for _, a := range report.Assumptions {
		if a.Source == "workload.yaml:region" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("got %d entries for the same (Kind,Source) pair repeated 6 times in one finding, want exactly 1 (deduplicated)", count)
	}
}

func TestReport_Validate(t *testing.T) {
	ir := &core.IR{}
	report := core.BuildReport("s1", 1, "<svg>test</svg>", ir, core.Workload{}, nil, core.Scorecard{VersionNumber: 1}, nil, core.PriceTable{}, nil)
	if err := report.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}
