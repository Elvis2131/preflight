package ingest_test

// PC-133: proves the three real Terraform shapes golden/aws has none of — inline
// identity policy, managed-policy attachment, and resource-based (bucket) policy —
// against ingest/testdata/iam-fixture, the only real coverage for these shapes.

import (
	"path/filepath"
	"testing"

	"preflight/ingest"
)

func loadIAMFixture(t *testing.T) *ingest.Result {
	t.Helper()
	reg := loadRegistry(t)
	result, err := ingest.Ingest(filepath.Join("testdata", "iam-fixture"), reg, 1)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if result.Insufficient != nil {
		t.Fatalf("got insufficient_model: %+v", result.Insufficient)
	}
	return &result
}

func TestIAM_InlineIdentityPolicy_WildcardAndConditionPreservedVerbatim(t *testing.T) {
	result := loadIAMFixture(t)

	var found bool
	for _, n := range result.IR.Nodes {
		if n.ID != "aws_iam_role.app" {
			continue
		}
		found = true
		if len(n.IAMIdentityPolicies) == 0 {
			t.Fatal("expected at least one identity policy attached to aws_iam_role.app")
		}
		var matched bool
		for _, doc := range n.IAMIdentityPolicies {
			if doc.ID != "aws_iam_role_policy.app_inline" {
				continue
			}
			matched = true
			if len(doc.Statements) != 1 {
				t.Fatalf("got %d statements, want 1", len(doc.Statements))
			}
			stmt := doc.Statements[0]
			if stmt.Sid != "ReadSecrets" || stmt.Effect != "Allow" {
				t.Errorf("got Sid=%q Effect=%q", stmt.Sid, stmt.Effect)
			}
			wantActions := []string{"secretsmanager:GetSecretValue", "secretsmanager:DescribeSecret"}
			if len(stmt.Action) != 2 || stmt.Action[0] != wantActions[0] || stmt.Action[1] != wantActions[1] {
				t.Errorf("Action = %v, want %v (a real JSON array, preserved verbatim)", stmt.Action, wantActions)
			}
			if len(stmt.Resource) != 1 || stmt.Resource[0] != "arn:aws:secretsmanager:eu-west-1:123456789012:secret:payments/*" {
				t.Errorf("Resource = %v, want the wildcard ARN preserved verbatim, not expanded", stmt.Resource)
			}
			if stmt.Condition == nil {
				t.Fatal("Condition is nil, want the real StringEquals block preserved")
			}
			se, ok := stmt.Condition["StringEquals"].(map[string]any)
			if !ok || se["aws:RequestedRegion"] != "eu-west-1" {
				t.Errorf("Condition[StringEquals] = %v, want aws:RequestedRegion=eu-west-1 preserved verbatim", stmt.Condition["StringEquals"])
			}
		}
		if !matched {
			t.Error("expected aws_iam_role_policy.app_inline among aws_iam_role.app's IAMIdentityPolicies")
		}
	}
	if !found {
		t.Fatal("expected aws_iam_role.app in the IR")
	}
}

func TestIAM_ManagedPolicyAttachment_ResolvesThroughAttachment(t *testing.T) {
	result := loadIAMFixture(t)

	for _, n := range result.IR.Nodes {
		if n.ID != "aws_iam_role.app" {
			continue
		}
		matched := false
		for _, doc := range n.IAMIdentityPolicies {
			if doc.ID == "aws_iam_policy.read_artifacts" {
				matched = true
				if len(doc.Statements) != 1 || len(doc.Statements[0].Action) != 1 || doc.Statements[0].Action[0] != "s3:GetObject" {
					t.Errorf("got %+v, want a single s3:GetObject statement", doc.Statements)
				}
			}
		}
		if !matched {
			t.Error("expected aws_iam_policy.read_artifacts (via aws_iam_role_policy_attachment) among aws_iam_role.app's IAMIdentityPolicies")
		}
	}
}

func TestIAM_ResourceBasedPolicy_AttachesToBucketNotIdentity(t *testing.T) {
	result := loadIAMFixture(t)

	var bucketFound bool
	for _, n := range result.IR.Nodes {
		if n.ID != "aws_s3_bucket.artifacts" {
			continue
		}
		bucketFound = true
		if n.IAMResourcePolicy == nil {
			t.Fatal("IAMResourcePolicy is nil, want the real bucket policy")
		}
		if n.IAMIdentityPolicies != nil {
			t.Errorf("IAMIdentityPolicies = %+v on a bucket, want nil — resource policies and identity policies are mutually exclusive", n.IAMIdentityPolicies)
		}
		stmt := n.IAMResourcePolicy.Statements[0]
		if stmt.Effect != "Deny" {
			t.Errorf("Effect = %q, want Deny", stmt.Effect)
		}
		if stmt.Principal != "*" {
			t.Errorf("Principal = %#v, want the bare string \"*\", preserved verbatim", stmt.Principal)
		}
		wantResources := []string{"arn:aws:s3:::payments-artifacts", "arn:aws:s3:::payments-artifacts/*"}
		if len(stmt.Resource) != 2 || stmt.Resource[0] != wantResources[0] || stmt.Resource[1] != wantResources[1] {
			t.Errorf("Resource = %v, want %v", stmt.Resource, wantResources)
		}
		b, ok := stmt.Condition["Bool"].(map[string]any)
		if !ok || b["aws:SecureTransport"] != "false" {
			t.Errorf("Condition[Bool] = %v, want aws:SecureTransport=false preserved verbatim", stmt.Condition["Bool"])
		}
	}
	if !bucketFound {
		t.Fatal("expected aws_s3_bucket.artifacts in the IR")
	}

	for _, n := range result.IR.Nodes {
		if n.ID == "aws_iam_role.app" && n.IAMResourcePolicy != nil {
			t.Error("aws_iam_role.app must not have an IAMResourcePolicy — that's the bucket's own")
		}
	}
}
