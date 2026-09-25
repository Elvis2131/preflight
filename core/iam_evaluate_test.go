package core_test

// PC-134's own conformance tests — each cites the exact AWS evaluation-logic
// statement it proves, per the Card's own instruction ("verified against the IAM
// policy evaluation documentation and cited in code and conformance tests").
// Fixtures are hand-built (this ticket's own scenarios, not golden/aws — PC-133's
// golden-bundle roles don't exercise every branch this engine has), following the
// same synthetic-fixture discipline PC-114/125/129 established.

import (
	"strings"
	"testing"

	"preflight/core"
)

func identityPolicy(id string, statements ...core.PolicyStatement) core.PolicyDocument {
	return core.PolicyDocument{ID: id, Version: "2012-10-17", Statements: statements, Provenance: syntheticProv()}
}

func identityNode(id string, docs ...core.PolicyDocument) core.Node {
	n := rt(id, core.NodeTypeIdentity)
	n.IAMIdentityPolicies = docs
	return n
}

// TestEvaluateIAMRequest_ImplicitDeny proves the documented default: "By default,
// all requests are implicitly denied" (reference_policies_evaluation-logic_policy-
// eval-denyallow.html) — an identity with no policy granting the action gets Deny,
// not not_assessable (the absence of any matching statement is a known fact).
func TestEvaluateIAMRequest_ImplicitDeny(t *testing.T) {
	role := identityNode("role", identityPolicy("policy",
		core.PolicyStatement{Effect: "Allow", Action: []string{"s3:GetObject"}, Resource: []string{"arn:aws:s3:::other-bucket/*"}},
	))
	ir := &core.IR{Nodes: []core.Node{role}}

	result := core.EvaluateIAMRequest(ir, core.IAMRequest{
		PrincipalID: "role",
		Action:      "s3:GetObject",
		ResourceARN: "arn:aws:s3:::my-bucket/key",
	}, syntheticProv())

	if result.Decision != core.IAMDecisionDeny {
		t.Fatalf("Decision = %q, want deny (implicit)", result.Decision)
	}
	if len(result.DecidingStatements) != 0 {
		t.Errorf("implicit deny should cite no deciding statement, got %+v", result.DecidingStatements)
	}
}

// TestEvaluateIAMRequest_IdentityAllow proves the documented rule: "If any statement
// in any applicable identity-based policies allows the requested action, the code
// evaluation continues [toward Allow]" (policy-eval-denyallow.html).
func TestEvaluateIAMRequest_IdentityAllow(t *testing.T) {
	role := identityNode("role", identityPolicy("policy",
		core.PolicyStatement{Sid: "ReadMyBucket", Effect: "Allow", Action: []string{"s3:GetObject"}, Resource: []string{"arn:aws:s3:::my-bucket/*"}},
	))
	ir := &core.IR{Nodes: []core.Node{role}}

	result := core.EvaluateIAMRequest(ir, core.IAMRequest{
		PrincipalID: "role",
		Action:      "s3:GetObject",
		ResourceARN: "arn:aws:s3:::my-bucket/key",
	}, syntheticProv())

	if result.Decision != core.IAMDecisionAllow {
		t.Fatalf("Decision = %q, want allow", result.Decision)
	}
	if len(result.DecidingStatements) != 1 || result.DecidingStatements[0].Sid != "ReadMyBucket" {
		t.Fatalf("DecidingStatements = %+v, want the ReadMyBucket statement named", result.DecidingStatements)
	}
}

// TestEvaluateIAMRequest_ExplicitDenyOverridesAllow proves: "An explicit deny
// overrides an explicit allow" (policy-eval-denyallow.html) — an identity policy
// granting the action, with a second statement explicitly denying the same action,
// must resolve to Deny, not Allow.
func TestEvaluateIAMRequest_ExplicitDenyOverridesAllow(t *testing.T) {
	role := identityNode("role", identityPolicy("policy",
		core.PolicyStatement{Sid: "AllowRead", Effect: "Allow", Action: []string{"s3:GetObject"}, Resource: []string{"arn:aws:s3:::my-bucket/*"}},
		core.PolicyStatement{Sid: "DenyRead", Effect: "Deny", Action: []string{"s3:GetObject"}, Resource: []string{"arn:aws:s3:::my-bucket/*"}},
	))
	ir := &core.IR{Nodes: []core.Node{role}}

	result := core.EvaluateIAMRequest(ir, core.IAMRequest{
		PrincipalID: "role",
		Action:      "s3:GetObject",
		ResourceARN: "arn:aws:s3:::my-bucket/key",
	}, syntheticProv())

	if result.Decision != core.IAMDecisionDeny {
		t.Fatalf("Decision = %q, want deny (explicit deny overrides allow)", result.Decision)
	}
	if len(result.DecidingStatements) != 1 || result.DecidingStatements[0].Sid != "DenyRead" {
		t.Fatalf("DecidingStatements = %+v, want the DenyRead statement named", result.DecidingStatements)
	}
}

// TestEvaluateIAMRequest_ResourcePolicyAllow proves: "If an action is allowed by an
// identity-based policy, a resource-based policy, or both, then AWS allows the
// action" (reference_policies_evaluation-logic.html, "Evaluating identity-based
// policies with resource-based policies") — an identity with NO identity policy at
// all is still allowed, purely via the resource's own bucket policy naming it.
func TestEvaluateIAMRequest_ResourcePolicyAllow(t *testing.T) {
	role := identityNode("role") // no identity policies at all
	bucket := rt("bucket", core.NodeTypeObjectStore)
	bucket.IAMResourcePolicy = &core.PolicyDocument{
		ID:      "bucket-policy",
		Version: "2012-10-17",
		Statements: []core.PolicyStatement{
			{Sid: "AllowRoleRead", Effect: "Allow", Principal: map[string]any{"AWS": "arn:aws:iam::111122223333:role/role"},
				Action: []string{"s3:GetObject"}, Resource: []string{"arn:aws:s3:::my-bucket/*"}},
		},
		Provenance: syntheticProv(),
	}
	ir := &core.IR{Nodes: []core.Node{role, bucket}}

	result := core.EvaluateIAMRequest(ir, core.IAMRequest{
		PrincipalID:         "role",
		Action:              "s3:GetObject",
		ResourceARN:         "arn:aws:s3:::my-bucket/key",
		ResourceID:          "bucket",
		PrincipalIdentifier: "arn:aws:iam::111122223333:role/role",
	}, syntheticProv())

	if result.Decision != core.IAMDecisionAllow {
		t.Fatalf("Decision = %q, want allow (resource-based policy grants it)", result.Decision)
	}
	if len(result.DecidingStatements) != 1 || result.DecidingStatements[0].Sid != "AllowRoleRead" {
		t.Fatalf("DecidingStatements = %+v, want the AllowRoleRead statement named", result.DecidingStatements)
	}
}

// TestEvaluateIAMRequest_ResourcePolicy_PrincipalMismatchNeverAllows proves the
// engine never treats "no principal identifier supplied" as "matches everyone" —
// IAMRequest's own documented contract.
func TestEvaluateIAMRequest_ResourcePolicy_PrincipalMismatchNeverAllows(t *testing.T) {
	role := identityNode("role")
	bucket := rt("bucket", core.NodeTypeObjectStore)
	bucket.IAMResourcePolicy = &core.PolicyDocument{
		ID: "bucket-policy",
		Statements: []core.PolicyStatement{
			{Effect: "Allow", Principal: map[string]any{"AWS": "arn:aws:iam::111122223333:role/someone-else"},
				Action: []string{"s3:GetObject"}, Resource: []string{"arn:aws:s3:::my-bucket/*"}},
		},
		Provenance: syntheticProv(),
	}
	ir := &core.IR{Nodes: []core.Node{role, bucket}}

	result := core.EvaluateIAMRequest(ir, core.IAMRequest{
		PrincipalID: "role",
		Action:      "s3:GetObject",
		ResourceARN: "arn:aws:s3:::my-bucket/key",
		ResourceID:  "bucket",
		// PrincipalIdentifier deliberately omitted.
	}, syntheticProv())

	if result.Decision != core.IAMDecisionDeny {
		t.Fatalf("Decision = %q, want deny — no principal identifier was supplied to match the resource policy's Principal", result.Decision)
	}
}

// TestEvaluateAssumeRole proves: "Role assumption requires the trust policy to allow
// the principal" (the Card's own conformance case) — both the allow and deny sides,
// against docs.aws.amazon.com's own documented Principal-in-trust-policy behavior.
func TestEvaluateAssumeRole(t *testing.T) {
	role := rt("role", core.NodeTypeIdentity)
	role.IAMTrustPolicy = &core.PolicyDocument{
		ID: "trust-policy",
		Statements: []core.PolicyStatement{
			{Sid: "AllowEC2", Effect: "Allow", Principal: map[string]any{"Service": "ec2.amazonaws.com"}, Action: []string{"sts:AssumeRole"}},
		},
		Provenance: syntheticProv(),
	}
	ir := &core.IR{Nodes: []core.Node{role}}

	allowed := core.EvaluateAssumeRole(ir, core.IAMAssumeRoleRequest{RoleID: "role", PrincipalIdentifier: "ec2.amazonaws.com"}, syntheticProv())
	if allowed.Decision != core.IAMDecisionAllow {
		t.Fatalf("ec2.amazonaws.com: Decision = %q, want allow", allowed.Decision)
	}

	denied := core.EvaluateAssumeRole(ir, core.IAMAssumeRoleRequest{RoleID: "role", PrincipalIdentifier: "lambda.amazonaws.com"}, syntheticProv())
	if denied.Decision != core.IAMDecisionDeny {
		t.Fatalf("lambda.amazonaws.com: Decision = %q, want deny (trust policy names only ec2.amazonaws.com)", denied.Decision)
	}
}

// TestEvaluateAssumeRole_NoTrustPolicyCaptured_NotAssessable proves the IR-honesty
// side of I4: a role this engine cannot find a trust policy for is not_assessable,
// never assumed-deniable (a real gap in what was captured, not evidence of a real
// restrictive trust policy).
func TestEvaluateAssumeRole_NoTrustPolicyCaptured_NotAssessable(t *testing.T) {
	role := rt("role", core.NodeTypeIdentity) // no IAMTrustPolicy at all
	ir := &core.IR{Nodes: []core.Node{role}}

	result := core.EvaluateAssumeRole(ir, core.IAMAssumeRoleRequest{RoleID: "role", PrincipalIdentifier: "ec2.amazonaws.com"}, syntheticProv())
	if result.Decision != core.IAMDecisionNotAssessable {
		t.Fatalf("Decision = %q, want not_assessable", result.Decision)
	}
}

// TestEvaluateIAMRequest_WildcardActionAndResource proves the documented grammar:
// "You can use multi-character match wildcards (*) and single-character match
// wildcards (?)" and "iam:*AccessKey*" style mid-string wildcards
// (reference_policies_elements_action.html) — plus the equivalent ARN-suffix
// wildcard form for Resource.
func TestEvaluateIAMRequest_WildcardActionAndResource(t *testing.T) {
	cases := []struct {
		name       string
		statements []core.PolicyStatement
		action     string
		resource   string
		wantAllow  bool
	}{
		{
			name:       "service wildcard s3:*",
			statements: []core.PolicyStatement{{Effect: "Allow", Action: []string{"s3:*"}, Resource: []string{"arn:aws:s3:::my-bucket/*"}}},
			action:     "s3:PutObject", resource: "arn:aws:s3:::my-bucket/key", wantAllow: true,
		},
		{
			name:       "mid-string wildcard iam:*AccessKey*",
			statements: []core.PolicyStatement{{Effect: "Allow", Action: []string{"iam:*AccessKey*"}, Resource: []string{"*"}}},
			action:     "iam:DeleteAccessKey", resource: "arn:aws:iam::111122223333:user/x", wantAllow: true,
		},
		{
			name:       "single-char wildcard ? matches exactly one character",
			statements: []core.PolicyStatement{{Effect: "Allow", Action: []string{"s3:Get?bject"}, Resource: []string{"*"}}},
			action:     "s3:GetXbject", resource: "arn:x", wantAllow: true,
		},
		{
			name:       "single-char wildcard ? does not match zero or two characters",
			statements: []core.PolicyStatement{{Effect: "Allow", Action: []string{"s3:Get?bject"}, Resource: []string{"*"}}},
			action:     "s3:GetXYbject", resource: "arn:x", wantAllow: false,
		},
		{
			name:       "action name is case-insensitive",
			statements: []core.PolicyStatement{{Effect: "Allow", Action: []string{"S3:GETOBJECT"}, Resource: []string{"*"}}},
			action:     "s3:GetObject", resource: "arn:x", wantAllow: true,
		},
		{
			name:       "ARN prefix wildcard does not match a sibling bucket",
			statements: []core.PolicyStatement{{Effect: "Allow", Action: []string{"s3:GetObject"}, Resource: []string{"arn:aws:s3:::my-bucket/*"}}},
			action:     "s3:GetObject", resource: "arn:aws:s3:::other-bucket/key", wantAllow: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			role := identityNode("role", identityPolicy("policy", tc.statements...))
			ir := &core.IR{Nodes: []core.Node{role}}
			result := core.EvaluateIAMRequest(ir, core.IAMRequest{PrincipalID: "role", Action: tc.action, ResourceARN: tc.resource}, syntheticProv())
			gotAllow := result.Decision == core.IAMDecisionAllow
			if gotAllow != tc.wantAllow {
				t.Errorf("Decision = %q, want allow=%v", result.Decision, tc.wantAllow)
			}
		})
	}
}

// TestEvaluateIAMRequest_UnsupportedCondition_NeverYieldsAllow is the Card's own
// stated test requirement, verbatim: "test proves it never yields allow." A
// statement with an unsupported condition operator (aws:MultiFactorAuthPresent via
// Bool, which this engine does not implement) must never resolve to Allow even
// though it is the ONLY statement that could grant the action.
func TestEvaluateIAMRequest_UnsupportedCondition_NeverYieldsAllow(t *testing.T) {
	role := identityNode("role", identityPolicy("policy",
		core.PolicyStatement{
			Sid: "MFAOnly", Effect: "Allow", Action: []string{"s3:GetObject"}, Resource: []string{"arn:aws:s3:::my-bucket/*"},
			Condition: map[string]any{"Bool": map[string]any{"aws:MultiFactorAuthPresent": "true"}},
		},
	))
	ir := &core.IR{Nodes: []core.Node{role}}

	result := core.EvaluateIAMRequest(ir, core.IAMRequest{
		PrincipalID: "role", Action: "s3:GetObject", ResourceARN: "arn:aws:s3:::my-bucket/key",
		Context: map[string]string{"aws:MultiFactorAuthPresent": "true"}, // even supplied and "true" — still not silently trusted
	}, syntheticProv())

	if result.Decision == core.IAMDecisionAllow {
		t.Fatalf("Decision = allow — an unsupported condition operator (Bool) must never be silently treated as satisfied to reach an allow")
	}
	if result.Decision != core.IAMDecisionNotAssessable {
		t.Fatalf("Decision = %q, want not_assessable", result.Decision)
	}
	if len(result.DecidingStatements) != 1 || !strings.Contains(result.DecidingStatements[0].Reason, "Bool") {
		t.Fatalf("DecidingStatements = %+v, want the Bool operator named in the reason", result.DecidingStatements)
	}
}

// TestEvaluateIAMRequest_UnsupportedConditionOnDeny_NeverSilentlyAllows proves the
// symmetric case: an unsupported condition on a DENY statement — with a separate,
// unconditional Allow present — must not resolve to Allow either, since the denying
// statement might genuinely apply.
func TestEvaluateIAMRequest_UnsupportedConditionOnDeny_NeverSilentlyAllows(t *testing.T) {
	role := identityNode("role", identityPolicy("policy",
		core.PolicyStatement{Sid: "BroadAllow", Effect: "Allow", Action: []string{"s3:GetObject"}, Resource: []string{"arn:aws:s3:::my-bucket/*"}},
		core.PolicyStatement{
			Sid: "ConditionalDeny", Effect: "Deny", Action: []string{"s3:GetObject"}, Resource: []string{"arn:aws:s3:::my-bucket/*"},
			Condition: map[string]any{"NumericLessThan": map[string]any{"s3:max-keys": "10"}},
		},
	))
	ir := &core.IR{Nodes: []core.Node{role}}

	result := core.EvaluateIAMRequest(ir, core.IAMRequest{PrincipalID: "role", Action: "s3:GetObject", ResourceARN: "arn:aws:s3:::my-bucket/key"}, syntheticProv())

	if result.Decision != core.IAMDecisionNotAssessable {
		t.Fatalf("Decision = %q, want not_assessable (the conditional deny might apply)", result.Decision)
	}
}

// TestEvaluateIAMRequest_SupportedConditionNarrowsAllow proves the engine actually
// evaluates the conditions it claims to support, not just detects the unsupported
// ones — StringEquals failing means the statement does not apply at all.
func TestEvaluateIAMRequest_SupportedConditionNarrowsAllow(t *testing.T) {
	role := identityNode("role", identityPolicy("policy",
		core.PolicyStatement{
			Effect: "Allow", Action: []string{"s3:GetObject"}, Resource: []string{"arn:aws:s3:::my-bucket/*"},
			Condition: map[string]any{"StringEquals": map[string]any{"aws:PrincipalTag/team": "payments"}},
		},
	))
	ir := &core.IR{Nodes: []core.Node{role}}

	allowed := core.EvaluateIAMRequest(ir, core.IAMRequest{
		PrincipalID: "role", Action: "s3:GetObject", ResourceARN: "arn:aws:s3:::my-bucket/key",
		Context: map[string]string{"aws:PrincipalTag/team": "payments"},
	}, syntheticProv())
	if allowed.Decision != core.IAMDecisionAllow {
		t.Fatalf("matching StringEquals: Decision = %q, want allow", allowed.Decision)
	}

	denied := core.EvaluateIAMRequest(ir, core.IAMRequest{
		PrincipalID: "role", Action: "s3:GetObject", ResourceARN: "arn:aws:s3:::my-bucket/key",
		Context: map[string]string{"aws:PrincipalTag/team": "other-team"},
	}, syntheticProv())
	if denied.Decision != core.IAMDecisionDeny {
		t.Fatalf("mismatched StringEquals: Decision = %q, want deny (implicit — condition not satisfied)", denied.Decision)
	}
}

// TestEvaluateIAMRequest_NonexistentPrincipal_NotAssessable and
// TestEvaluateIAMRequest_NonIdentityPrincipal_NotAssessable cover the structural
// input-validation edge the Card's acceptance criteria imply but don't name outright
// — never a silent Deny standing in for "this query doesn't even make sense."
func TestEvaluateIAMRequest_NonexistentPrincipal_NotAssessable(t *testing.T) {
	ir := &core.IR{}
	result := core.EvaluateIAMRequest(ir, core.IAMRequest{PrincipalID: "ghost", Action: "s3:GetObject"}, syntheticProv())
	if result.Decision != core.IAMDecisionNotAssessable {
		t.Fatalf("Decision = %q, want not_assessable", result.Decision)
	}
}

func TestEvaluateIAMRequest_NonIdentityPrincipal_NotAssessable(t *testing.T) {
	ir := &core.IR{Nodes: []core.Node{rt("bucket", core.NodeTypeObjectStore)}}
	result := core.EvaluateIAMRequest(ir, core.IAMRequest{PrincipalID: "bucket", Action: "s3:GetObject"}, syntheticProv())
	if result.Decision != core.IAMDecisionNotAssessable {
		t.Fatalf("Decision = %q, want not_assessable — %q is not an identity node", result.Decision, "bucket")
	}
}

// TestEvaluateIAMRequest_Validate is a basic Validate() smoke test, matching every
// other Validate()-carrying core result type's own test coverage.
func TestEvaluateIAMRequest_Validate(t *testing.T) {
	role := identityNode("role", identityPolicy("policy", core.PolicyStatement{Effect: "Allow", Action: []string{"s3:GetObject"}}))
	ir := &core.IR{Nodes: []core.Node{role}}
	result := core.EvaluateIAMRequest(ir, core.IAMRequest{PrincipalID: "role", Action: "s3:GetObject"}, syntheticProv())
	if err := result.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}
