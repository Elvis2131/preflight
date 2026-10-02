package core_test

// PC-157: an IAM identity policy the engine cannot read must make the role's IAM evaluation
// not_assessable, never an implicit "grants nothing". And a policy written as jsonencode() of
// constants (how Terraform authors write one) is read. Every case goes through real HCL ingest.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"preflight/core"
	"preflight/ingest"
	awsprovider "preflight/providers/aws"
)

func iamBundleIR(t *testing.T, tf string) *core.IR {
	t.Helper()
	dir := t.TempDir()
	src := tf + `
# minimum viable graph
resource "aws_lb" "front" { name = "front" }
resource "aws_db_instance" "d" { identifier = "d" }
`
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	reg, err := awsprovider.Load()
	if err != nil {
		t.Fatal(err)
	}
	res, err := ingest.Ingest(dir, reg, 1)
	if err != nil || res.IR == nil {
		t.Fatalf("Ingest: %v %+v", err, res.Insufficient)
	}
	return res.IR
}

const roleBlock = `
resource "aws_iam_role" "app" {
  name               = "app"
  assume_role_policy = "{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Principal\":{\"Service\":\"ec2.amazonaws.com\"},\"Action\":\"sts:AssumeRole\"}]}"
}
`

func wildcardProbe(ir *core.IR) core.IAMEvaluationResult {
	return core.EvaluateIAMRequest(ir, core.IAMRequest{PrincipalID: "aws_iam_role.app", Action: "*", ResourceARN: "*"}, core.NewProvenance(core.KindDerived, "test"))
}

func TestIAMJsonencodePolicy_IsRead(t *testing.T) {
	admin := roleBlock + `
resource "aws_iam_policy" "p" {
  name   = "p"
  policy = jsonencode({
    Version   = "2012-10-17"
    Statement = [{ Effect = "Allow", Action = "*", Resource = "*" }]
  })
}
resource "aws_iam_role_policy_attachment" "a" {
  role       = aws_iam_role.app.name
  policy_arn = aws_iam_policy.p.arn
}`
	if got := wildcardProbe(iamBundleIR(t, admin)).Decision; got != core.IAMDecisionAllow {
		t.Errorf("a jsonencode()d Action * / Resource * policy must be read and allowed, got %s", got)
	}

	scoped := roleBlock + `
resource "aws_iam_role_policy" "p" {
  name   = "p"
  role   = aws_iam_role.app.id
  policy = jsonencode({
    Version   = "2012-10-17"
    Statement = [{ Effect = "Allow", Action = ["s3:GetObject"], Resource = "arn:aws:s3:::my-bucket/*" }]
  })
}`
	if got := wildcardProbe(iamBundleIR(t, scoped)).Decision; got != core.IAMDecisionDeny {
		t.Errorf("a scoped literal policy grants no wildcard: want deny, got %s", got)
	}
}

// Negative controls: each way a policy can be unreadable must be not_assessable, and must stay so even
// when another readable policy would otherwise decide the request (an unread statement could hold an
// explicit Deny).
func TestIAMUnreadablePolicy_IsNotAssessable_NeverAPass(t *testing.T) {
	cases := map[string]string{
		"policy references a resource (ARN not resolvable statically)": roleBlock + `
resource "aws_sqs_queue" "q" { name = "q" }
resource "aws_iam_role_policy" "p" {
  name   = "p"
  role   = aws_iam_role.app.id
  policy = jsonencode({ Version = "2012-10-17", Statement = [{ Effect = "Allow", Action = ["sqs:SendMessage"], Resource = aws_sqs_queue.q.arn }] })
}`,
		"policy comes from a variable": roleBlock + `
resource "aws_iam_role_policy" "p" {
  name   = "p"
  role   = aws_iam_role.app.id
  policy = var.policy
}`,
		"policy is not valid IAM JSON": roleBlock + `
resource "aws_iam_role_policy" "p" {
  name   = "p"
  role   = aws_iam_role.app.id
  policy = "not json"
}`,
		"attached managed policy is defined outside the bundle": roleBlock + `
resource "aws_iam_role_policy_attachment" "a" {
  role       = aws_iam_role.app.name
  policy_arn = "arn:aws:iam::aws:policy/AdministratorAccess"
}`,
	}
	for name, tf := range cases {
		t.Run(name, func(t *testing.T) {
			res := wildcardProbe(iamBundleIR(t, tf))
			if res.Decision != core.IAMDecisionNotAssessable {
				t.Fatalf("decision = %s, want not_assessable", res.Decision)
			}
			if !strings.Contains(strings.Join(res.Reasoning, " "), "could not be read") {
				t.Errorf("reasoning %v should say the policy could not be read", res.Reasoning)
			}
		})
	}

	// A readable wildcard policy next to an unreadable one is still not_assessable: an explicit Deny in the
	// unread document could override the Allow.
	mixed := roleBlock + `
resource "aws_iam_role_policy" "admin" {
  name   = "admin"
  role   = aws_iam_role.app.id
  policy = jsonencode({ Version = "2012-10-17", Statement = [{ Effect = "Allow", Action = "*", Resource = "*" }] })
}
resource "aws_iam_role_policy" "unread" {
  name   = "unread"
  role   = aws_iam_role.app.id
  policy = var.other
}`
	if got := wildcardProbe(iamBundleIR(t, mixed)).Decision; got != core.IAMDecisionNotAssessable {
		t.Errorf("a readable Allow beside an unreadable policy must be not_assessable, got %s", got)
	}
}

// The unresolved marker survives a JSON round trip (the IR is stored and re-read as JSON).
func TestIAMUnresolvedMarker_SurvivesJSONRoundTrip(t *testing.T) {
	ir := &core.IR{Nodes: []core.Node{{
		ID: "aws_iam_role.app", Type: core.NodeTypeIdentity, Resolution: core.ResolutionKnown,
		RawAttributes: map[string]any{"unresolved_identity_policies": []any{"p: the policy document is not a static value"}},
		Provenance:    core.NewProvenance(core.KindStated, "test"),
	}}}
	if got := wildcardProbe(ir).Decision; got != core.IAMDecisionNotAssessable {
		t.Errorf("a []any marker (after a JSON round trip) must still be honoured, got %s", got)
	}
}

// golden/aws-broken's seeded defect 7 is now detected, and golden/aws's own policy (which references
// resource ARNs) is honestly not_assessable rather than the vacuous "satisfied" it used to report.
func TestIAMGolden_Defect7DetectedAndGoodBundleHonest(t *testing.T) {
	reg, err := awsprovider.Load()
	if err != nil {
		t.Fatal(err)
	}
	statusFor := func(dir string) string {
		res, err := ingest.Ingest(filepath.Join("..", "golden", dir), reg, 1)
		if err != nil || res.IR == nil {
			t.Fatalf("Ingest %s: %v", dir, err)
		}
		f := findFinding(core.IAMLeastPrivilegeFindings(res.IR), "finding.compliance.iam-least-privilege-wildcard-admin.aws_iam_role.payments_app")
		if f == nil {
			t.Fatalf("%s: no wildcard-admin finding for payments_app", dir)
		}
		if f.Outcome.State == core.AssessmentStateNotAssessable {
			return "not_assessable"
		}
		v, _ := f.Outcome.Value.(string)
		return v
	}
	if got := statusFor("aws-broken"); got != string(core.ComplianceUnsatisfied) {
		t.Errorf("aws-broken payments_app = %q, want unsatisfied (seeded defect 7)", got)
	}
	if got := statusFor("aws"); got != "not_assessable" {
		t.Errorf("aws payments_app = %q, want not_assessable (its policy references resource ARNs)", got)
	}
}

// PC-158: multi_az selects the RDS rate (Multi-AZ is roughly twice Single-AZ). A value ingest cannot read must
// make the component cost_unknown, not be priced at the single-AZ default.
func TestCost_UnreadableMultiAZ_IsCostUnknownNotSingleAZ(t *testing.T) {
	table := core.PriceTable{SnapshotID: "t", Rows: []core.PriceRow{
		{Service: "AmazonRDS", Unit: "Hrs", Price: 0.45, Currency: "USD", SKUAttributes: map[string]string{"instanceType": "db.r6g.xlarge", "databaseEngine": "PostgreSQL", "deploymentOption": "Single-AZ"}},
		{Service: "AmazonRDS", Unit: "Hrs", Price: 0.90, Currency: "USD", SKUAttributes: map[string]string{"instanceType": "db.r6g.xlarge", "databaseEngine": "PostgreSQL", "deploymentOption": "Multi-AZ"}},
	}}
	cases := map[string]struct {
		attr string
		want core.CostDecision
	}{
		"multi_az = true":                 {`multi_az = true`, core.CostPriced},
		"multi_az = false":                {`multi_az = false`, core.CostPriced},
		"multi_az absent (default false)": {``, core.CostPriced},
		"multi_az from a variable":        {`multi_az = var.ha`, core.CostUnknown},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			ir := iamBundleIR(t, `resource "aws_db_instance" "main" {
  identifier     = "main"
  instance_class = "db.r6g.xlarge"
  engine         = "postgres"
  `+c.attr+`
}`)
			report := core.ComputeCost(ir, table, []string{"eu-west-1"}, core.NewProvenance(core.KindDerived, "test"))
			for _, comp := range report.Components {
				if comp.NodeID != "aws_db_instance.main" {
					continue
				}
				if comp.Decision != c.want {
					t.Fatalf("decision = %s (%s), want %s", comp.Decision, comp.Reason, c.want)
				}
				if name == "multi_az = true" && comp.MonthlyAmount < 600 {
					t.Errorf("multi-AZ must be priced at the Multi-AZ rate (0.90/h), got %.2f/month", comp.MonthlyAmount)
				}
				return
			}
			t.Fatal("no cost component for the database")
		})
	}
}
