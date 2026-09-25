package core_test

// PC-135's third acceptance criterion: "At least two least-privilege controls
// produce evidence from IAM decisions; a not_assessable IAM decision makes the
// control not_assessable — tested."

import (
	"testing"

	"preflight/core"
)

func findFinding(findings []core.Finding, id string) *core.Finding {
	for i := range findings {
		if findings[i].ID == id {
			return &findings[i]
		}
	}
	return nil
}

func TestIAMLeastPrivilegeFindings_WildcardAdmin_Unsatisfied(t *testing.T) {
	role := identityNode("role", identityPolicy("policy",
		core.PolicyStatement{Effect: "Allow", Action: []string{"*"}, Resource: []string{"*"}},
	))
	ir := &core.IR{Nodes: []core.Node{role}}

	findings := core.IAMLeastPrivilegeFindings(ir)
	f := findFinding(findings, "finding.compliance.iam-least-privilege-wildcard-admin.role")
	if f == nil {
		t.Fatal("expected a wildcard-admin finding for role")
	}
	if f.Outcome.Value != string(core.ComplianceUnsatisfied) {
		t.Fatalf("Outcome.Value = %q, want %q", f.Outcome.Value, core.ComplianceUnsatisfied)
	}
}

func TestIAMLeastPrivilegeFindings_ScopedPolicy_Satisfied(t *testing.T) {
	role := identityNode("role", identityPolicy("policy",
		core.PolicyStatement{Effect: "Allow", Action: []string{"s3:GetObject"}, Resource: []string{"arn:aws:s3:::my-bucket/*"}},
	))
	ir := &core.IR{Nodes: []core.Node{role}}

	findings := core.IAMLeastPrivilegeFindings(ir)
	f := findFinding(findings, "finding.compliance.iam-least-privilege-wildcard-admin.role")
	if f == nil {
		t.Fatal("expected a wildcard-admin finding for role")
	}
	if f.Outcome.Value != string(core.ComplianceSatisfied) {
		t.Fatalf("Outcome.Value = %q, want %q — a scoped policy does not grant */*", f.Outcome.Value, core.ComplianceSatisfied)
	}
}

func TestIAMLeastPrivilegeFindings_AssumableByAnyPrincipal_Unsatisfied(t *testing.T) {
	role := rt("role", core.NodeTypeIdentity)
	role.IAMTrustPolicy = &core.PolicyDocument{
		ID:         "trust",
		Statements: []core.PolicyStatement{{Effect: "Allow", Principal: "*", Action: []string{"sts:AssumeRole"}}},
		Provenance: syntheticProv(),
	}
	ir := &core.IR{Nodes: []core.Node{role}}

	findings := core.IAMLeastPrivilegeFindings(ir)
	f := findFinding(findings, "finding.compliance.iam-least-privilege-assumable-by-any-principal.role")
	if f == nil {
		t.Fatal("expected an assumable-by-any-principal finding for role")
	}
	if f.Outcome.Value != string(core.ComplianceUnsatisfied) {
		t.Fatalf("Outcome.Value = %q, want %q", f.Outcome.Value, core.ComplianceUnsatisfied)
	}
}

func TestIAMLeastPrivilegeFindings_ScopedTrustPolicy_Satisfied(t *testing.T) {
	role := rt("role", core.NodeTypeIdentity)
	role.IAMTrustPolicy = &core.PolicyDocument{
		ID: "trust",
		Statements: []core.PolicyStatement{
			{Effect: "Allow", Principal: map[string]any{"Service": "ec2.amazonaws.com"}, Action: []string{"sts:AssumeRole"}},
		},
		Provenance: syntheticProv(),
	}
	ir := &core.IR{Nodes: []core.Node{role}}

	findings := core.IAMLeastPrivilegeFindings(ir)
	f := findFinding(findings, "finding.compliance.iam-least-privilege-assumable-by-any-principal.role")
	if f == nil {
		t.Fatal("expected an assumable-by-any-principal finding for role")
	}
	if f.Outcome.Value != string(core.ComplianceSatisfied) {
		t.Fatalf("Outcome.Value = %q, want %q — trust policy names only ec2.amazonaws.com", f.Outcome.Value, core.ComplianceSatisfied)
	}
}

// TestIAMLeastPrivilegeFindings_UnsupportedCondition_NotAssessable_NeverSatisfied is
// the Card's own explicit rule, verbatim: "Controls whose evaluation hits a
// not_assessable IAM decision are themselves not_assessable, never satisfied."
func TestIAMLeastPrivilegeFindings_UnsupportedCondition_NotAssessable_NeverSatisfied(t *testing.T) {
	role := identityNode("role", identityPolicy("policy",
		core.PolicyStatement{
			Effect: "Allow", Action: []string{"*"}, Resource: []string{"*"},
			Condition: map[string]any{"Bool": map[string]any{"aws:MultiFactorAuthPresent": "true"}},
		},
	))
	ir := &core.IR{Nodes: []core.Node{role}}

	findings := core.IAMLeastPrivilegeFindings(ir)
	f := findFinding(findings, "finding.compliance.iam-least-privilege-wildcard-admin.role")
	if f == nil {
		t.Fatal("expected a wildcard-admin finding for role")
	}
	if f.Outcome.State != core.AssessmentStateNotAssessable {
		t.Fatalf("Outcome.State = %q, want not_assessable — an unresolved condition must never be reported as satisfied", f.Outcome.State)
	}
	if f.Outcome.Reason == "" {
		t.Error("Outcome.Reason is empty, want the unsupported-condition reason recorded")
	}
}

func TestIAMLeastPrivilegeFindings_SkipsNonIdentityNodes(t *testing.T) {
	ir := &core.IR{Nodes: []core.Node{rt("bucket", core.NodeTypeObjectStore)}}
	findings := core.IAMLeastPrivilegeFindings(ir)
	if len(findings) != 0 {
		t.Fatalf("got %d findings for a non-identity node, want 0", len(findings))
	}
}
