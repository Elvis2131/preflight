package ingest_test

// PC-133: trust policy parsing, hand-verified against the golden AWS bundle's own
// REAL assume_role_policy documents (golden/aws/eks.tf) — real data, not a fixture
// invented for this test.

import (
	"path/filepath"
	"testing"

	"preflight/core"
	"preflight/ingest"
)

func TestIAM_TrustPolicy_GoldenBundle_HandVerified(t *testing.T) {
	reg := loadRegistry(t)
	result, err := ingest.Ingest(filepath.Join("..", "golden", "aws"), reg, 1)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if result.Insufficient != nil {
		t.Fatalf("got insufficient_model: %+v", result.Insufficient)
	}

	var role *core.Node
	for i := range result.IR.Nodes {
		if result.IR.Nodes[i].ID == "aws_iam_role.eks_cluster" {
			role = &result.IR.Nodes[i]
		}
	}
	if role == nil {
		t.Fatal("expected aws_iam_role.eks_cluster in the IR")
	}
	if role.IAMTrustPolicy == nil {
		t.Fatal("IAMTrustPolicy is nil, want a real parsed trust policy")
	}
	if len(role.IAMTrustPolicy.Statements) != 1 {
		t.Fatalf("got %d statements, want 1", len(role.IAMTrustPolicy.Statements))
	}
	stmt := role.IAMTrustPolicy.Statements[0]
	if stmt.Effect != "Allow" {
		t.Errorf("Effect = %q, want Allow", stmt.Effect)
	}
	// golden/aws/eks.tf's own real JSON: "Action": "sts:AssumeRole" (a bare string,
	// not an array) — proves the real AWS "string or array" shape is handled, not
	// just the array case.
	if len(stmt.Action) != 1 || stmt.Action[0] != "sts:AssumeRole" {
		t.Errorf("Action = %v, want [sts:AssumeRole] (a bare JSON string normalized to a one-element slice)", stmt.Action)
	}
	// "Principal": { "Service": "eks.amazonaws.com" } — an object, preserved verbatim.
	principal, ok := stmt.Principal.(map[string]any)
	if !ok {
		t.Fatalf("Principal = %#v (%T), want a map (preserved verbatim)", stmt.Principal, stmt.Principal)
	}
	if principal["Service"] != "eks.amazonaws.com" {
		t.Errorf("Principal[\"Service\"] = %v, want eks.amazonaws.com", principal["Service"])
	}
}

// TestIAM_AWSManagedPolicyAttachment_RealStatedGap proves the golden bundle's own
// real aws_iam_role_policy_attachment resources naming an AWS-managed policy ARN
// (e.g. "arn:aws:iam::aws:policy/AmazonEKSClusterPolicy" — not a resource declared in
// this bundle at all) correctly resolve to no identity policy at all — a real, stated
// gap (nothing to merge, never a guess at what that managed policy contains), not a
// crash or a silently fabricated document.
func TestIAM_AWSManagedPolicyAttachment_RealStatedGap(t *testing.T) {
	reg := loadRegistry(t)
	result, err := ingest.Ingest(filepath.Join("..", "golden", "aws"), reg, 1)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	for _, n := range result.IR.Nodes {
		if n.ID == "aws_iam_role.eks_cluster" {
			if len(n.IAMIdentityPolicies) != 0 {
				t.Errorf("got %d identity policies, want 0 — this role's only attachments name AWS-managed policy ARNs, not resources declared in this bundle", len(n.IAMIdentityPolicies))
			}
		}
	}
	// The attachment resources themselves must still be recognized (edge-only, not
	// out-of-vocabulary) even though they resolve to no in-bundle policy document.
	foundEdgeOnly := false
	for _, e := range result.EdgeOnlyResources {
		if e.ResourceType == "aws_iam_role_policy_attachment" {
			foundEdgeOnly = true
		}
	}
	if !foundEdgeOnly {
		t.Error("expected aws_iam_role_policy_attachment to be tracked as edge-only, not out-of-vocabulary")
	}
	for _, oov := range result.OutOfVocabulary {
		if oov.ResourceType == "aws_iam_role_policy_attachment" || oov.ResourceType == "aws_iam_policy" {
			t.Errorf("%s must not be out-of-vocabulary — it is a real, recognized resource", oov.ResourceType)
		}
	}
}

func TestIAM_TrustPolicy_AbsentForNonRoleNodes(t *testing.T) {
	reg := loadRegistry(t)
	result, err := ingest.Ingest(filepath.Join("..", "golden", "aws"), reg, 1)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	for _, n := range result.IR.Nodes {
		if n.Type != core.NodeTypeIdentity && n.IAMTrustPolicy != nil {
			t.Errorf("%s (%s): IAMTrustPolicy is non-nil on a non-identity node, want nil", n.ID, n.Type)
		}
	}
}
