// This file is PC-118's own acceptance criterion, verbatim: "Budget declared as a hard
// requirement produces a failing finding when exceeded; as a preference, a trade-off
// entry — both tested." Kept separate from findings_builder.go's own BuildFindings
// since cost findings depend on a CostReport (PC-117), a fact BuildFindings' own
// signature (ir, workload) has no way to receive — server.assessFromResult calls this
// alongside BuildFindings, appending its result the same way
// BuildUnsupportedRouteFindings already is.
package core

import "fmt"

// BuildCostBudgetFindings returns zero or one Finding, using the real, existing
// ComplianceStatus vocabulary (satisfied/unsatisfied/not_assessable) so ComputeDelta's
// own compliance-shaped classification applies to it for free, no separate cost-delta
// mechanism needed for the budget-compliance case specifically.
//
// Only a HARD-priority budget ever produces a Finding at all. A PREFERENCE-priority
// budget being exceeded is never a Finding — it is a trade-off, not a regression (the
// Card's own instruction) — reported instead via CostReport.BudgetExceeded, which the
// caller surfaces in the cost dimension directly. No declared budget requirement at
// all -> no finding (nothing to check).
func BuildCostBudgetFindings(cost *CostReport, requirements []Requirement) []Finding {
	amount, priority, ok := findCostBudgetRequirement(requirements)
	if !ok || priority != PriorityHard {
		return nil
	}

	prov := NewProvenance(KindDerived, "core/cost:budget-check")

	var status ComplianceStatus
	var rationale string
	switch {
	case cost == nil:
		status = ComplianceNotAssessable
		rationale = "no pricing snapshot was available to check the declared hard cost budget against"
	case cost.BudgetExceeded:
		status = ComplianceUnsatisfied
		rationale = fmt.Sprintf("monthly priced total $%.2f exceeds the declared hard budget of $%.2f", cost.PricedTotal, amount)
	default:
		status = ComplianceSatisfied
		rationale = fmt.Sprintf("monthly priced total $%.2f is within the declared hard budget of $%.2f", cost.PricedTotal, amount)
	}

	outcome := AssessmentEnvelope{State: AssessmentStateAssessed, Value: string(status), Provenance: prov}
	if status == ComplianceNotAssessable {
		outcome = NotAssessable[any](rationale, prov).ToEnvelope()
	}

	return []Finding{{
		ID:    "finding.cost.budget-compliance",
		Title: "Declared monthly cost budget (hard requirement)",
		Dimensions: FailureMode{
			Trigger:            "the assessed architecture's priced monthly cost exceeds a declared hard budget",
			AffectedComponents: nil,
			Detection:          DetectionModeled,
			Impact:             NotAssessable[any]("impact dimension not evaluated by this compliance check", prov).ToEnvelope(),
			Likelihood:         DeriveLikelihood(prov).ToEnvelope(),
			Detectability:      DeriveDetectability(DetectionModeled, prov).ToEnvelope(),
			Recoverability: Recoverability{
				FailoverPathExists: NotAssessable[any]("recoverability not evaluated by this compliance check", prov).ToEnvelope(),
				RPOFeasible:        NotAssessable[any]("recoverability not evaluated by this compliance check", prov).ToEnvelope(),
			},
		},
		Evidence: []EvidenceRef{
			{Description: rationale},
		},
		Outcome: outcome,
	}}
}
