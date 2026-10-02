package server_test

// PC-28: the agent-iteration acceptance test. This is the test that determines whether
// everything built since PC-7 adds up to something an agent can actually use, not just
// something that compiles and passes unit tests: it drives a MUTABLE WORKING COPY of
// golden/aws-broken through the real author -> evaluate -> modify -> re-evaluate loop
// (CLAUDE.md §3), applying a deterministic scripted "stub agent" fix each round based on
// which findings server.Assess currently reports unsatisfied, and asserts the loop
// actually converges within a written-down bound.
//
// The bound (N=5) is a decision made BEFORE this file was written, not discovered here
// by trial and error — see docs/PC-28-ITERATION-BOUND.md for the full reasoning. This
// file only consumes that decision; it does not re-derive it.
//
// Scope, stated rather than silently expanded: PC-28's Card lists three acceptance
// criteria. This test is the first — "a CI-deterministic test exists that runs the
// author-evaluate-modify loop against a known-broken bundle and asserts convergence." A
// separate live-agent run recorded for the demo (criterion 3) is explicitly PC-29's job,
// not built here — there is no LLM client wired up yet (ADR-005/PC-77), and stubbing one
// in to satisfy this test would misrepresent what PC-28 itself claims to deliver.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"preflight/server"
)

// maxConvergenceIterations is docs/PC-28-ITERATION-BOUND.md's N=5, copied here as the
// single source the test asserts against — not re-derived, not tuned to whatever this
// implementation happens to take.
const maxConvergenceIterations = 5

// copyBundleToTempDir gives each test run its own mutable working copy of
// golden/aws-broken. golden/aws-broken itself (the checked-in fixture bundle) is never
// written to.
func copyBundleToTempDir(t *testing.T, srcDir string) string {
	t.Helper()
	dstDir := t.TempDir()

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", srcDir, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue // golden/aws-broken is flat; a subdirectory here would be unexpected
		}
		data, err := os.ReadFile(filepath.Join(srcDir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(dstDir, e.Name()), data, 0o644); err != nil {
			t.Fatalf("write %s: %v", e.Name(), err)
		}
	}
	return dstDir
}

// unsatisfiedFindingIDs returns the IDs of every finding whose Outcome.Value is
// "unsatisfied" — the stub agent's own view of "what still needs fixing," driven off
// the same response shape a real agent would see, not off internal test knowledge of
// which defect is which.
func unsatisfiedFindingIDs(resp server.AssessResponse) []string {
	var ids []string
	for _, f := range resp.Findings {
		if f.Outcome.Value == "unsatisfied" {
			ids = append(ids, f.ID)
		}
	}
	return ids
}

// applyScriptedFix is the "stub agent": a deterministic, non-LLM stand-in that maps a
// specific unsatisfied finding ID to the exact Terraform edit that resolves it. This is
// intentionally NOT a general Terraform-patching engine — PC-28's Card frames the test
// as proving the LOOP converges given correct fixes, not as proving an agent can
// discover the fixes itself (that is what the real reason/ + runnerd loop, and PC-29's
// live-agent demo, are for).
func applyScriptedFix(t *testing.T, workDir, findingID string) (applied bool) {
	t.Helper()

	switch findingID {
	case "finding.compliance.nat-gateway-redundancy":
		fixNATGatewayRedundancy(t, workDir)
		return true

	case "finding.compliance.rds-storage-encryption.aws_db_instance.payments",
		"finding.compliance.rds-rpo-feasibility.aws_db_instance.payments":
		fixRDSMultiAZAndEncryption(t, workDir)
		return true

	case "finding.compliance.iam-least-privilege-wildcard-admin.aws_iam_role.payments_app":
		fixIAMWildcardAdmin(t, workDir)
		return true

	default:
		return false
	}
}

// fixIAMWildcardAdmin resolves golden/aws-broken's defect 7 (PC-157: detected since ingest reads
// jsonencode() policy documents) by replacing the Action "*" / Resource "*" statement with the two
// scoped statements golden/aws's own iam.tf uses (copied from it, not invented). Those statements
// reference resource ARNs, which the engine cannot resolve statically, so the fixed role's least-
// privilege result is not_assessable rather than satisfied: honest, and not "unsatisfied", so the loop
// converges.
func fixIAMWildcardAdmin(t *testing.T, workDir string) {
	t.Helper()
	path := filepath.Join(workDir, "iam.tf")
	content := readFile(t, path)
	content = replaceExactlyOnce(t, content,
		`      {
        Sid      = "AppAccess"
        Effect   = "Allow"
        Action   = "*"
        Resource = "*"
      }`,
		`      {
        Sid    = "SettlementQueueAccess"
        Effect = "Allow"
        Action = [
          "sqs:SendMessage",
          "sqs:ReceiveMessage",
          "sqs:DeleteMessage",
          "sqs:GetQueueAttributes"
        ]
        Resource = aws_sqs_queue.settlement.arn
      },
      {
        Sid    = "PaymentsDataKeyUse"
        Effect = "Allow"
        Action = [
          "kms:Decrypt",
          "kms:GenerateDataKey"
        ]
        Resource = aws_kms_key.payments.arn
      }`,
		"payments_app wildcard statement")
	writeFile(t, path, content)
}

// fixNATGatewayRedundancy resolves golden/aws-broken's defect 1 by rewriting
// network.tf's single-NAT-gateway section into the same three-AZ shape golden/aws
// itself uses (verified against golden/aws/network.tf directly, not invented here).
func fixNATGatewayRedundancy(t *testing.T, workDir string) {
	t.Helper()
	path := filepath.Join(workDir, "network.tf")
	content := readFile(t, path)

	if strings.Contains(content, `aws_nat_gateway" "nat_b"`) {
		return // already fixed by an earlier iteration
	}

	oldEIP := `resource "aws_eip" "nat_a" {
  domain = "vpc"

  tags = {
    Name = "payments-nat-eip-a"
  }
}`
	newEIP := oldEIP + `

resource "aws_eip" "nat_b" {
  domain = "vpc"

  tags = {
    Name = "payments-nat-eip-b"
  }
}

resource "aws_eip" "nat_c" {
  domain = "vpc"

  tags = {
    Name = "payments-nat-eip-c"
  }
}`
	content = replaceExactlyOnce(t, content, oldEIP, newEIP, "nat_a EIP block")

	oldGW := `resource "aws_nat_gateway" "nat_a" {
  allocation_id = aws_eip.nat_a.id
  subnet_id     = aws_subnet.public_a.id
  depends_on    = [aws_internet_gateway.payments]

  tags = {
    Name = "payments-nat-a"
  }
}`
	newGW := oldGW + `

resource "aws_nat_gateway" "nat_b" {
  allocation_id = aws_eip.nat_b.id
  subnet_id     = aws_subnet.public_b.id
  depends_on    = [aws_internet_gateway.payments]

  tags = {
    Name = "payments-nat-b"
  }
}

resource "aws_nat_gateway" "nat_c" {
  allocation_id = aws_eip.nat_c.id
  subnet_id     = aws_subnet.public_c.id
  depends_on    = [aws_internet_gateway.payments]

  tags = {
    Name = "payments-nat-c"
  }
}`
	content = replaceExactlyOnce(t, content, oldGW, newGW, "nat_a gateway block")

	content = replaceExactlyOnce(t, content,
		`resource "aws_route_table" "private_b" {
  vpc_id = aws_vpc.payments.id

  route {
    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.nat_a.id
  }`,
		`resource "aws_route_table" "private_b" {
  vpc_id = aws_vpc.payments.id

  route {
    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.nat_b.id
  }`,
		"private_b route table")

	content = replaceExactlyOnce(t, content,
		`resource "aws_route_table" "private_c" {
  vpc_id = aws_vpc.payments.id

  route {
    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.nat_a.id
  }`,
		`resource "aws_route_table" "private_c" {
  vpc_id = aws_vpc.payments.id

  route {
    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.nat_c.id
  }`,
		"private_c route table")

	writeFile(t, path, content)
}

// fixRDSMultiAZAndEncryption resolves golden/aws-broken's defect 2 by flipping both
// seeded-false attributes to true — the exact fix golden/aws-broken's own rds.tf
// comment names as the defect.
func fixRDSMultiAZAndEncryption(t *testing.T, workDir string) {
	t.Helper()
	path := filepath.Join(workDir, "rds.tf")
	content := readFile(t, path)

	content = strings.Replace(content, "storage_encrypted     = false", "storage_encrypted     = true", 1)
	content = strings.Replace(content, "multi_az = false", "multi_az = true", 1)

	writeFile(t, path, content)
}

func replaceExactlyOnce(t *testing.T, content, old, new, label string) string {
	t.Helper()
	count := strings.Count(content, old)
	if count != 1 {
		t.Fatalf("expected exactly 1 occurrence of %s, found %d — golden/aws-broken/network.tf may have changed shape since this fix was scripted", label, count)
	}
	return strings.Replace(content, old, new, 1)
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestAgentIterationLoop_ConvergesOnKnownBrokenBundle is PC-28's acceptance test.
func TestAgentIterationLoop_ConvergesOnKnownBrokenBundle(t *testing.T) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	workDir := copyBundleToTempDir(t, "../golden/aws-broken")
	workloadPath, err := filepath.Abs("../golden/workload.yaml")
	if err != nil {
		t.Fatalf("abs workload path: %v", err)
	}

	const sessionID = "pc-28-acceptance"
	var last server.AssessResponse

	for iteration := 1; iteration <= maxConvergenceIterations; iteration++ {
		resp, err := server.Assess(store, server.AssessRequest{
			SessionID:    sessionID,
			BundleDir:    workDir,
			WorkloadPath: workloadPath,
		})
		if err != nil {
			t.Fatalf("iteration %d: Assess: %v", iteration, err)
		}
		last = resp

		unsatisfied := unsatisfiedFindingIDs(resp)
		if len(unsatisfied) == 0 {
			t.Logf("converged after %d iteration(s) (bound was %d)", iteration, maxConvergenceIterations)
			return
		}

		fixedAny := false
		for _, id := range unsatisfied {
			if applyScriptedFix(t, workDir, id) {
				fixedAny = true
			}
		}
		if !fixedAny {
			t.Fatalf("iteration %d: %d finding(s) unsatisfied but the stub agent has no scripted fix for any of them: %v",
				iteration, len(unsatisfied), unsatisfied)
		}
	}

	t.Fatalf("did not converge within the written-down bound of %d iterations (docs/PC-28-ITERATION-BOUND.md); still unsatisfied: %v",
		maxConvergenceIterations, unsatisfiedFindingIDs(last))
}
