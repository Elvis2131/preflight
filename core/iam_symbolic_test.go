package core_test

// PC-159: IAM policies that reference resources in the bundle become assessable. A reference like
// aws_s3_bucket.data.arn identifies WHICH resource a statement targets; the concrete ARN is only known after
// apply and the engine does not need it. Everything else stays not_assessable, naming the reference.

import (
	"strings"
	"testing"

	"preflight/core"
)

const symbolicBase = `
resource "aws_s3_bucket" "data" { bucket = "data" }
resource "aws_sqs_queue" "q" { name = "q" }
resource "aws_sqs_queue" "other" { name = "other" }
resource "aws_iam_role" "app" {
  name               = "app"
  assume_role_policy = "{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Principal\":{\"Service\":\"ec2.amazonaws.com\"},\"Action\":\"sts:AssumeRole\"}]}"
}
`

func rolePolicy(statements string) string {
	return symbolicBase + `
resource "aws_iam_role_policy" "p" {
  name   = "p"
  role   = aws_iam_role.app.id
  policy = jsonencode({ Version = "2012-10-17", Statement = [` + statements + `] })
}`
}

func probe(ir *core.IR, action, arn, resourceID string) core.IAMEvaluationResult {
	return core.EvaluateIAMRequest(ir, core.IAMRequest{PrincipalID: "aws_iam_role.app", Action: action, ResourceARN: arn, ResourceID: resourceID},
		core.NewProvenance(core.KindDerived, "test"))
}

func TestSymbolic_InBundleArnAndObjectSuffixResolve_WithDerivedProvenance(t *testing.T) {
	ir := iamBundleIR(t, rolePolicy(`
    { Effect = "Allow", Action = ["sqs:SendMessage"], Resource = aws_sqs_queue.q.arn },
    { Effect = "Allow", Action = ["s3:GetObject"], Resource = "${aws_s3_bucket.data.arn}/*" }`))
	var role core.Node
	for _, n := range ir.Nodes {
		if n.ID == "aws_iam_role.app" {
			role = n
		}
	}
	if len(role.IAMIdentityPolicies) != 1 {
		t.Fatalf("got %d identity policies, want 1 (the policy must be read): unresolved=%v", len(role.IAMIdentityPolicies), role.RawAttributes["unresolved_identity_policies"])
	}
	doc := role.IAMIdentityPolicies[0]
	if got := doc.Statements[0].Resource; len(got) != 1 || got[0] != "preflight-ref:aws_sqs_queue.q" {
		t.Errorf("queue statement resource = %v, want the symbolic reference preflight-ref:aws_sqs_queue.q", got)
	}
	if got := doc.Statements[1].Resource; len(got) != 1 || got[0] != "preflight-ref:aws_s3_bucket.data/*" {
		t.Errorf("object statement resource = %v, want preflight-ref:aws_s3_bucket.data/* (objects within the bucket)", got)
	}
	if doc.Provenance.Kind != core.KindDerived || !strings.Contains(doc.Provenance.Reason, "aws_s3_bucket.data") || !strings.Contains(doc.Provenance.Reason, "aws_sqs_queue.q") {
		t.Errorf("provenance = %+v, want derived, naming the resolved references", doc.Provenance)
	}
}

// Wildcard detection never depends on the concrete ARN: it is the same on symbolic and literal references.
func TestSymbolic_WildcardDetectionIsTheSameOnSymbolicAndLiteralReferences(t *testing.T) {
	scoped := iamBundleIR(t, rolePolicy(`{ Effect = "Allow", Action = "*", Resource = aws_s3_bucket.data.arn }`))
	if got := probe(scoped, "*", "*", "").Decision; got != core.IAMDecisionDeny {
		t.Errorf("every action on ONE specific resource is not administrator access: want deny, got %s", got)
	}
	admin := iamBundleIR(t, rolePolicy(`
    { Effect = "Allow", Action = ["sqs:SendMessage"], Resource = aws_sqs_queue.q.arn },
    { Effect = "Allow", Action = "*", Resource = "*" }`))
	if got := probe(admin, "*", "*", "").Decision; got != core.IAMDecisionAllow {
		t.Errorf("a literal Action * / Resource * beside symbolic statements must still be detected: got %s", got)
	}
	// And the literal-reference twin of the scoped case.
	literal := iamBundleIR(t, rolePolicy(`{ Effect = "Allow", Action = "*", Resource = "arn:aws:s3:::data" }`))
	if got := probe(literal, "*", "*", "").Decision; got != core.IAMDecisionDeny {
		t.Errorf("literal twin: want deny, got %s", got)
	}
}

// Real requests: a symbolic statement answers for the node it names, and is uncertain where the request does not say.
func TestSymbolic_RequestsAreDecidedOnlyWhereTheResourceIsKnown(t *testing.T) {
	ir := iamBundleIR(t, rolePolicy(`
    { Effect = "Allow", Action = ["sqs:SendMessage"], Resource = aws_sqs_queue.q.arn },
    { Effect = "Allow", Action = ["s3:GetObject"], Resource = "${aws_s3_bucket.data.arn}/*" }`))
	cases := []struct {
		name, action, arn, id string
		want                  core.IAMDecision
	}{
		{"the named queue", "sqs:SendMessage", "arn:aws:sqs:eu-west-1:1:q", "aws_sqs_queue.q", core.IAMDecisionAllow},
		{"a different resource", "sqs:SendMessage", "arn:aws:sqs:eu-west-1:1:other", "aws_sqs_queue.other", core.IAMDecisionDeny},
		{"an object inside the named bucket", "s3:GetObject", "arn:aws:s3:::data/key", "aws_s3_bucket.data", core.IAMDecisionAllow},
		{"the bucket itself is not 'objects within'", "s3:GetObject", "arn:aws:s3:::data", "aws_s3_bucket.data", core.IAMDecisionDeny},
		{"an action the role lacks", "sqs:DeleteMessage", "arn:aws:sqs:eu-west-1:1:q", "aws_sqs_queue.q", core.IAMDecisionDeny},
		{"an ARN only: whether it is the queue is unknown", "sqs:SendMessage", "arn:aws:sqs:eu-west-1:1:q", "", core.IAMDecisionNotAssessable},
		{"an ARN only, but the action does not match: no doubt", "sqs:DeleteMessage", "arn:aws:sqs:eu-west-1:1:q", "", core.IAMDecisionDeny},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := probe(ir, c.action, c.arn, c.id).Decision; got != c.want {
				t.Errorf("decision = %s, want %s", got, c.want)
			}
		})
	}
}

// Out-of-bundle and unrecognised expressions stay not_assessable, and the reason names the reference.
func TestSymbolic_OutOfBundleAndUnrecognisedExpressions_StayNotAssessable_NamingTheReference(t *testing.T) {
	cases := map[string]struct{ statement, mention string }{
		"a data source":                         {`{ Effect = "Allow", Action = ["s3:GetObject"], Resource = data.aws_s3_bucket.ext.arn }`, "data.aws_s3_bucket.ext"},
		"a variable":                            {`{ Effect = "Allow", Action = ["s3:GetObject"], Resource = var.bucket_arn }`, "var.bucket_arn"},
		"a resource that is not in this bundle": {`{ Effect = "Allow", Action = ["s3:GetObject"], Resource = aws_s3_bucket.ghost.arn }`, "aws_s3_bucket.ghost"},
		"an attribute other than arn":           {`{ Effect = "Allow", Action = ["s3:GetObject"], Resource = aws_s3_bucket.data.id }`, "aws_s3_bucket.data.id"},
		"a reference combined with other text":  {`{ Effect = "Allow", Action = ["s3:GetObject"], Resource = "${aws_s3_bucket.data.arn}-backup" }`, "text other than the /* object suffix"},
		"a reference in a principal":            {`{ Effect = "Allow", Principal = { AWS = aws_iam_role.app.arn }, Action = ["s3:GetObject"], Resource = "*" }`, "outside its Resource element"},
		"a reference in a condition":            {`{ Effect = "Allow", Action = ["s3:GetObject"], Resource = "*", Condition = { StringEquals = { "aws:SourceArn" = aws_sqs_queue.q.arn } } }`, "outside its Resource element"},
		"a reference in NotResource":            {`{ Effect = "Allow", Action = ["s3:GetObject"], NotResource = aws_s3_bucket.data.arn }`, "NotResource"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			res := probe(iamBundleIR(t, rolePolicy(c.statement)), "*", "*", "")
			if res.Decision != core.IAMDecisionNotAssessable {
				t.Fatalf("decision = %s, want not_assessable", res.Decision)
			}
			if joined := strings.Join(res.Reasoning, " "); !strings.Contains(joined, c.mention) {
				t.Errorf("reasoning %q should name %q", joined, c.mention)
			}
		})
	}
}

// An explicit Deny on a symbolic resource is honoured, and a symbolic Deny the request cannot place makes the
// answer not_assessable rather than letting an Allow through.
func TestSymbolic_ExplicitDenyOnASymbolicResource(t *testing.T) {
	ir := iamBundleIR(t, rolePolicy(`
    { Effect = "Allow", Action = ["s3:*"], Resource = "*" },
    { Effect = "Deny", Action = ["s3:DeleteObject"], Resource = "${aws_s3_bucket.data.arn}/*" }`))
	if got := probe(ir, "s3:DeleteObject", "arn:aws:s3:::data/k", "aws_s3_bucket.data").Decision; got != core.IAMDecisionDeny {
		t.Errorf("an explicit Deny on objects within the bucket must win: got %s", got)
	}
	if got := probe(ir, "s3:DeleteObject", "arn:aws:s3:::data/k", "").Decision; got != core.IAMDecisionNotAssessable {
		t.Errorf("a Deny that might apply (the request does not say which resource) must not be skipped: got %s", got)
	}
	if got := probe(ir, "s3:GetObject", "arn:aws:s3:::data/k", "aws_s3_bucket.data").Decision; got != core.IAMDecisionAllow {
		t.Errorf("another action on the same object is allowed: got %s", got)
	}
}
