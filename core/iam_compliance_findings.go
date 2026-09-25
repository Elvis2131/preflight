// This file is PC-135's third integration point, verbatim from the Card:
// "least-privilege and access-control controls use IAM engine output as evidence,
// e.g. flag wildcard actions on sensitive resources, roles assumable by overly broad
// principals. Controls whose evaluation hits a not_assessable IAM decision are
// themselves not_assessable, never satisfied."
//
// Both controls below call PC-134's EvaluateIAMRequest/EvaluateAssumeRole directly —
// zero IAM evaluation logic is reimplemented here, per the Card's own "consumers call
// the PC-134 evaluator" boundary rule (core/iam_boundary_test.go proves this with a
// grep-based test, mirroring PC-115/126's own "no Sizing reference" structural
// checks).
//
// SCOPE NOTE, stated rather than silent: "sensitive resource" is generalized to the
// resource-agnostic worst case ("*") rather than a specific bucket/table ARN, because
// core.Node has no ARN field (PC-133's own stated scope) — there is no real per-node
// ARN this engine could plug into EvaluateIAMRequest's own ResourceARN parameter
// without guessing one. Testing "*"/"*" (full wildcard action on every resource) is a
// real, honest least-privilege check in its own right (the worst-case over-broad
// grant), not a placeholder standing in for the ARN-specific version this IR cannot
// yet express.
package core

// iamWildcardAdminFinding calls EvaluateIAMRequest(role, "*", "*") — reusing PC-134
// verbatim — to ask "does this role's own identity policy grant every action on every
// resource." An Allow decision is a real, structural over-permission (unsatisfied); a
// Deny decision (implicit or explicit) is satisfied; a not_assessable IAM decision
// (e.g. an unsupported Condition guarding the only matching statement) makes the
// control itself not_assessable, never satisfied — the Card's own explicit rule.
func iamWildcardAdminFinding(ir *IR, role Node) Finding {
	prov := NewProvenance(KindDerived, "core/iam_evaluate:least-privilege-wildcard-admin:"+role.ID)
	result := EvaluateIAMRequest(ir, IAMRequest{PrincipalID: role.ID, Action: "*", ResourceARN: "*"}, prov)

	status, rationale := iamDecisionToCompliance(result, "no identity policy statement grants every action (\"*\") on every resource (\"*\")",
		"an identity policy statement grants every action (\"*\") on every resource (\"*\") — full administrative access")

	nodeID := role.ID
	attr := "iam_identity_policies"
	outcome := AssessmentEnvelope{State: AssessmentStateAssessed, Value: string(status), Provenance: prov}
	if status == ComplianceNotAssessable {
		outcome = NotAssessable[any](rationale, prov).ToEnvelope()
	}

	return Finding{
		ID:    "finding.compliance.iam-least-privilege-wildcard-admin." + role.ID,
		Title: "IAM role " + role.ID + ": least-privilege check for wildcard administrative access",
		Dimensions: FailureMode{
			Trigger:            "a compromised or misused credential for this role has unrestricted access to every resource",
			AffectedComponents: []string{nodeID},
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
			{NodeID: &nodeID, Attribute: &attr, Description: rationale},
		},
		Outcome: outcome,
	}
}

// iamAssumableByAnyPrincipalFinding calls EvaluateAssumeRole(role, "*") — again PC-134
// verbatim — to ask "would this role's own trust policy allow ANY principal to assume
// it." Passing "*" as the identifier only ever matches a trust policy statement whose
// own Principal is itself literally "*" (or an "AWS"/"Service"/"Federated" value of
// "*") — principalApplies's own glob match against a concrete, non-wildcard ARN never
// matches the literal string "*" — so an Allow here is real evidence of an overly
// broad trust policy, not a false positive from the test identifier itself.
func iamAssumableByAnyPrincipalFinding(ir *IR, role Node) Finding {
	prov := NewProvenance(KindDerived, "core/iam_evaluate:least-privilege-assumable-by-any-principal:"+role.ID)
	result := EvaluateAssumeRole(ir, IAMAssumeRoleRequest{RoleID: role.ID, PrincipalIdentifier: "*"}, prov)

	status, rationale := iamDecisionToCompliance(result, "this role's trust policy does not allow an unrestricted (\"*\") principal to assume it",
		"this role's trust policy allows an unrestricted (\"*\") principal to assume it")

	nodeID := role.ID
	attr := "iam_trust_policy"
	outcome := AssessmentEnvelope{State: AssessmentStateAssessed, Value: string(status), Provenance: prov}
	if status == ComplianceNotAssessable {
		outcome = NotAssessable[any](rationale, prov).ToEnvelope()
	}

	return Finding{
		ID:    "finding.compliance.iam-least-privilege-assumable-by-any-principal." + role.ID,
		Title: "IAM role " + role.ID + ": least-privilege check for an overly broad trust policy",
		Dimensions: FailureMode{
			Trigger:            "any AWS principal, not just the intended one, can assume this role",
			AffectedComponents: []string{nodeID},
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
			{NodeID: &nodeID, Attribute: &attr, Description: rationale},
		},
		Outcome: outcome,
	}
}

// iamDecisionToCompliance maps an IAMEvaluationResult onto this codebase's
// ComplianceStatus vocabulary (core/compliance.go) — Allow is the violation being
// checked FOR, so it maps to Unsatisfied; Deny (implicit or explicit) means the
// violation was not found, Satisfied; not_assessable propagates as-is, the Card's own
// explicit "never satisfied" rule.
func iamDecisionToCompliance(result IAMEvaluationResult, satisfiedRationale, unsatisfiedRationale string) (ComplianceStatus, string) {
	switch result.Decision {
	case IAMDecisionAllow:
		return ComplianceUnsatisfied, unsatisfiedRationale
	case IAMDecisionNotAssessable:
		return ComplianceNotAssessable, joinReasoning(result.Reasoning)
	default: // IAMDecisionDeny
		return ComplianceSatisfied, satisfiedRationale
	}
}

// IAMLeastPrivilegeFindings runs both least-privilege controls against every identity
// node in ir — BuildFindings' own caller, generic over any IR (not hardcoded to
// golden's own role IDs), matching PC-29's "any node of this type" discipline.
func IAMLeastPrivilegeFindings(ir *IR) []Finding {
	var findings []Finding
	for _, n := range ir.Nodes {
		if n.Type != NodeTypeIdentity {
			continue
		}
		findings = append(findings, iamWildcardAdminFinding(ir, n))
		findings = append(findings, iamAssumableByAnyPrincipalFinding(ir, n))
	}
	return findings
}
