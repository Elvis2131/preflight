package core_test

// PC-118: BuildCostBudgetFindings' own acceptance criterion, verbatim: "Budget
// declared as a hard requirement produces a failing finding when exceeded; as a
// preference, a trade-off entry — both tested."

import (
	"testing"

	"preflight/core"
)

func hardBudgetRequirement(amount float64) []core.Requirement {
	return []core.Requirement{{ID: core.CostBudgetRequirementID, Value: amount, Priority: core.PriorityHard}}
}

func preferenceBudgetRequirement(amount float64) []core.Requirement {
	rank := 1
	return []core.Requirement{{ID: core.CostBudgetRequirementID, Value: amount, Priority: core.PriorityPreference, Rank: &rank}}
}

func reportWithTotal(total float64) core.CostReport {
	return core.CostReport{SnapshotID: "s1", PricedTotal: total, Currency: "USD"}
}

func TestApplyCostBudget_HardExceeded(t *testing.T) {
	report := core.ApplyCostBudget(reportWithTotal(1000), hardBudgetRequirement(500))
	if report.BudgetUSD == nil || *report.BudgetUSD != 500 {
		t.Fatalf("BudgetUSD = %v, want 500", report.BudgetUSD)
	}
	if report.BudgetPriority != core.PriorityHard {
		t.Errorf("BudgetPriority = %q, want hard", report.BudgetPriority)
	}
	if !report.BudgetExceeded {
		t.Error("BudgetExceeded = false, want true (1000 > 500)")
	}
}

func TestApplyCostBudget_NoRequirementDeclared_Unchanged(t *testing.T) {
	report := core.ApplyCostBudget(reportWithTotal(1000), nil)
	if report.BudgetUSD != nil {
		t.Errorf("BudgetUSD = %v, want nil (no budget requirement declared)", report.BudgetUSD)
	}
}

func TestBuildCostBudgetFindings_HardExceeded_ProducesFailingFinding(t *testing.T) {
	report := core.ApplyCostBudget(reportWithTotal(1000), hardBudgetRequirement(500))
	findings := core.BuildCostBudgetFindings(&report, hardBudgetRequirement(500))
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want exactly 1", len(findings))
	}
	status, ok := findings[0].Outcome.Value.(string)
	if !ok || status != string(core.ComplianceUnsatisfied) {
		t.Fatalf("Outcome.Value = %v, want %q (a hard budget exceeded IS a failing finding)", findings[0].Outcome.Value, core.ComplianceUnsatisfied)
	}
}

func TestBuildCostBudgetFindings_HardWithinBudget_ProducesSatisfiedFinding(t *testing.T) {
	report := core.ApplyCostBudget(reportWithTotal(200), hardBudgetRequirement(500))
	findings := core.BuildCostBudgetFindings(&report, hardBudgetRequirement(500))
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want exactly 1", len(findings))
	}
	status, ok := findings[0].Outcome.Value.(string)
	if !ok || status != string(core.ComplianceSatisfied) {
		t.Fatalf("Outcome.Value = %v, want %q", findings[0].Outcome.Value, core.ComplianceSatisfied)
	}
}

// TestBuildCostBudgetFindings_PreferenceExceeded_NeverAFinding is PC-118's own
// instruction, directly: "A cost increase is not automatically a regression ... as a
// preference, a trade-off entry" — never a compliance Finding at all.
func TestBuildCostBudgetFindings_PreferenceExceeded_NeverAFinding(t *testing.T) {
	report := core.ApplyCostBudget(reportWithTotal(1000), preferenceBudgetRequirement(500))
	if !report.BudgetExceeded {
		t.Fatal("expected BudgetExceeded=true (1000 > 500) — the trade-off fact must still be recorded on the report itself")
	}
	findings := core.BuildCostBudgetFindings(&report, preferenceBudgetRequirement(500))
	if len(findings) != 0 {
		t.Fatalf("got %d findings, want 0 — a preference budget being exceeded is a trade-off, never a compliance finding: %+v", len(findings), findings)
	}
}

func TestBuildCostBudgetFindings_NoBudgetDeclared_NoFinding(t *testing.T) {
	report := reportWithTotal(1000)
	findings := core.BuildCostBudgetFindings(&report, nil)
	if len(findings) != 0 {
		t.Fatalf("got %d findings, want 0 (no budget requirement was declared at all)", len(findings))
	}
}

func TestBuildCostBudgetFindings_HardBudgetButNoCostData_NotAssessable(t *testing.T) {
	findings := core.BuildCostBudgetFindings(nil, hardBudgetRequirement(500))
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want exactly 1 (not_assessable — a declared hard budget with no cost data to check it against)", len(findings))
	}
	if findings[0].Outcome.State != core.AssessmentStateNotAssessable {
		t.Errorf("Outcome.State = %v, want not_assessable", findings[0].Outcome.State)
	}
}
