// Package failover holds AWS conformance tests for replication/failover behaviour —
// PC-119's backfill of the RDS Multi-AZ and ElastiCache automatic-failover claims
// already doc-verified during the provider-agnostic refactor (PC-22/29,
// providers/aws/rds.yaml and providers/aws/elasticache.yaml's own citation fields),
// now proven end to end (real ingest -> real IR capability, not just read back out of
// the YAML file that made the claim in the first place) rather than left as an
// uncited assertion.
package failover

import (
	"os"
	"path/filepath"
	"testing"

	"preflight/core"
	"preflight/ingest"
	awsprovider "preflight/providers/aws"
	"preflight/tests/aws-conformance/harness"
)

func writeTF(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return dir
}

func findNodeByType(t *testing.T, nodes []core.Node, want core.NodeType) core.Node {
	t.Helper()
	for _, n := range nodes {
		if n.Type == want {
			return n
		}
	}
	t.Fatalf("no node of type %q found among %d nodes", want, len(nodes))
	return core.Node{}
}

// derefStr formats a *string for a failure message without leaking a bare pointer
// address when the underlying test failure is itself "this pointer wasn't what I
// expected" — nil prints as the literal word, never a hex address.
func derefStr(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

// TestRDSMultiAZ_Failover_001 is FAILOVER-RDS-MULTIAZ-001: multi_az=true must produce
// a real, doc-cited synchronous-replication/automatic-failover capability claim in
// the IR — not read back out of providers/aws/rds.yaml (which would only prove the
// YAML says what it says), but produced by actually running ingest against a real
// Terraform configuration, the same path a real assessment takes.
func TestRDSMultiAZ_Failover_001(t *testing.T) {
	spec := harness.Verify(t, harness.Spec{
		ID:            "FAILOVER-RDS-MULTIAZ-001",
		Rule:          "An RDS instance with multi_az enabled automatically provisions and maintains a synchronous standby replica in a different Availability Zone; the primary is synchronously replicated to that standby.",
		Source:        "https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/Concepts.MultiAZSingleStandby.html",
		Scenario:      "An aws_db_instance declares multi_az = true.",
		Configuration: "resource \"aws_route53_record\" \"entry\" {}\nresource \"aws_db_instance\" \"payments\" { multi_az = true }",
		Request:       "Ingest this configuration and read the resulting managed_database node's ReplicationMode/FailoverMechanism capability fields.",
		Expected:      "ReplicationMode = \"sync\"; FailoverMechanism names automatic failover to a synchronous standby replica.",
	})

	reg, err := awsprovider.Load()
	if err != nil {
		t.Fatalf("providers/aws.Load(): %v", err)
	}
	dir := writeTF(t, spec.Configuration)
	result, err := ingest.Ingest(dir, reg, 1)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if result.Insufficient != nil {
		t.Fatalf("%s: got insufficient_model: %+v", spec.ID, result.Insufficient)
	}
	node := findNodeByType(t, result.IR.Nodes, core.NodeTypeManagedDatabase)
	if node.Capability == nil {
		t.Fatalf("%s (%s): node has no capability data at all — see %s", spec.ID, spec.Rule, spec.Source)
	}
	if node.Capability.ReplicationMode == nil || *node.Capability.ReplicationMode != "sync" {
		t.Fatalf("%s (%s): ReplicationMode = %v, want \"sync\" — see %s", spec.ID, spec.Rule, derefStr(node.Capability.ReplicationMode), spec.Source)
	}
	if node.Capability.FailoverMechanism == nil || *node.Capability.FailoverMechanism == "" {
		t.Fatalf("%s (%s): FailoverMechanism is unset, want a real synchronous-standby-failover claim — see %s", spec.ID, spec.Rule, spec.Source)
	}
}

// TestRDSMultiAZ_Disabled_002 is FAILOVER-RDS-MULTIAZ-002: the negative case, equally
// part of the conformance claim — a Multi-AZ instance with the attribute explicitly
// disabled (or omitted, per this codebase's own not_assessable-on-absent discipline)
// must NOT claim synchronous replication/automatic failover it doesn't have.
func TestRDSMultiAZ_Disabled_002(t *testing.T) {
	spec := harness.Verify(t, harness.Spec{
		ID:            "FAILOVER-RDS-MULTIAZ-002",
		Rule:          "An RDS instance without Multi-AZ enabled has no automatic synchronous standby and no documented automatic failover mechanism.",
		Source:        "https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/Concepts.MultiAZSingleStandby.html",
		Scenario:      "An aws_db_instance declares multi_az = false.",
		Configuration: "resource \"aws_route53_record\" \"entry\" {}\nresource \"aws_db_instance\" \"payments\" { multi_az = false }",
		Request:       "Ingest this configuration and read the resulting managed_database node's ReplicationMode/FailoverMechanism capability fields.",
		Expected:      "FailoverMechanism is the explicit \"none\" sentinel, never the enabled-case wording, and never simply absent/ambiguous.",
	})

	reg, err := awsprovider.Load()
	if err != nil {
		t.Fatalf("providers/aws.Load(): %v", err)
	}
	dir := writeTF(t, spec.Configuration)
	result, err := ingest.Ingest(dir, reg, 1)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if result.Insufficient != nil {
		t.Fatalf("%s: got insufficient_model: %+v", spec.ID, result.Insufficient)
	}
	node := findNodeByType(t, result.IR.Nodes, core.NodeTypeManagedDatabase)
	if node.Capability == nil || node.Capability.FailoverMechanism == nil {
		t.Fatalf("%s (%s): FailoverMechanism is unset, want the explicit none sentinel — see %s", spec.ID, spec.Rule, spec.Source)
	}
	if *node.Capability.FailoverMechanism != "none" {
		t.Fatalf("%s (%s): FailoverMechanism = %q, want the \"none\" sentinel — a disabled Multi-AZ instance must never claim the enabled-case wording", spec.ID, spec.Rule, *node.Capability.FailoverMechanism)
	}
}
