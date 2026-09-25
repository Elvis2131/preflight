// This file is PC-126's own acceptance criterion, verbatim: "Bottlenecks become
// findings in the four-dimension schema (PC-17), so they flow into delta and report
// automatically." Uses the existing ComplianceStatus vocabulary (satisfied/
// unsatisfied/not_assessable) for the same reason PC-118's own budget finding does —
// the pre-existing compliance delta engine (ComputeDelta) then classifies
// improvement/regression for free, no second delta mechanism needed.
package core

import "fmt"

// BuildLoadFindings produces one finding per component ComputeComponentLoad reported
// — satisfied (within declared capacity), unsatisfied (over capacity — a real,
// evidenced bottleneck), or not_assessable (no declared capacity for that node type).
func BuildLoadFindings(loads []ComponentLoad, prov Provenance) []Finding {
	findings := make([]Finding, 0, len(loads))
	for _, l := range loads {
		findings = append(findings, buildOneLoadFinding(l, prov))
	}
	return findings
}

func buildOneLoadFinding(l ComponentLoad, prov Provenance) Finding {
	var status ComplianceStatus
	var rationale string
	switch {
	case l.Utilization == nil:
		status = ComplianceNotAssessable
		rationale = l.NotAssessableReason
	case *l.Utilization > 1.0:
		status = ComplianceUnsatisfied
		rationale = fmt.Sprintf("offered load %.1f rps exceeds declared capacity %.1f rps (%.0f%% utilisation)", l.OfferedRPS, *l.Capacity, *l.Utilization*100)
	default:
		status = ComplianceSatisfied
		rationale = fmt.Sprintf("offered load %.1f rps is within declared capacity %.1f rps (%.0f%% utilisation)", l.OfferedRPS, *l.Capacity, *l.Utilization*100)
	}

	outcome := AssessmentEnvelope{State: AssessmentStateAssessed, Value: string(status), Provenance: prov}
	if status == ComplianceNotAssessable {
		outcome = NotAssessable[any](rationale, prov).ToEnvelope()
	}

	nodeID := l.NodeID
	return Finding{
		ID:    "finding.load.bottleneck." + l.NodeID,
		Title: "Offered load vs. declared capacity: " + l.NodeID,
		Dimensions: FailureMode{
			Trigger:            "declared peak traffic exceeds this component's own declared capacity",
			AffectedComponents: []string{nodeID},
			Detection:          DetectionModeled,
			Impact:             NotAssessable[any]("impact dimension not evaluated by this structural check", prov).ToEnvelope(),
			Likelihood:         DeriveLikelihood(prov).ToEnvelope(),
			Detectability:      DeriveDetectability(DetectionModeled, prov).ToEnvelope(),
			Recoverability: Recoverability{
				FailoverPathExists: NotAssessable[any]("recoverability not evaluated by this structural check", prov).ToEnvelope(),
				RPOFeasible:        NotAssessable[any]("recoverability not evaluated by this structural check", prov).ToEnvelope(),
			},
		},
		Evidence: []EvidenceRef{
			{NodeID: &nodeID, Description: rationale},
		},
		Outcome: outcome,
	}
}
