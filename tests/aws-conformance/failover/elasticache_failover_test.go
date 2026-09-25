package failover

import (
	"testing"

	"preflight/core"
	"preflight/ingest"
	awsprovider "preflight/providers/aws"
	"preflight/tests/aws-conformance/harness"
)

// TestElastiCacheFailover_001 is FAILOVER-ELASTICACHE-001: an ElastiCache replication
// group with automatic_failover_enabled=true must produce a real, doc-cited
// asynchronous-replication/automatic-promotion capability claim in the IR — proven end
// to end via real ingest, not read back out of providers/aws/elasticache.yaml.
func TestElastiCacheFailover_001(t *testing.T) {
	spec := harness.Verify(t, harness.Spec{
		ID:            "FAILOVER-ELASTICACHE-001",
		Rule:          "An ElastiCache replication group with automatic failover enabled promotes the read replica with the least replication lag to primary on primary failure; replication itself is asynchronous.",
		Source:        "https://docs.aws.amazon.com/AmazonElastiCache/latest/red-ug/AutoFailover.html",
		Scenario:      "An aws_elasticache_replication_group declares automatic_failover_enabled = true.",
		Configuration: "resource \"aws_route53_record\" \"entry\" {}\nresource \"aws_elasticache_replication_group\" \"payments\" { automatic_failover_enabled = true }",
		Request:       "Ingest this configuration and read the resulting cache node's ReplicationMode/FailoverMechanism capability fields.",
		Expected:      "ReplicationMode = \"async\"; FailoverMechanism names automatic promotion of a replica to primary.",
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
	node := findNodeByType(t, result.IR.Nodes, core.NodeTypeCache)
	if node.Capability == nil || node.Capability.ReplicationMode == nil || *node.Capability.ReplicationMode != "async" {
		var got string
		if node.Capability != nil {
			got = derefStr(node.Capability.ReplicationMode)
		}
		t.Fatalf("%s (%s): ReplicationMode = %q, want \"async\" — see %s", spec.ID, spec.Rule, got, spec.Source)
	}
	if node.Capability.FailoverMechanism == nil || *node.Capability.FailoverMechanism == "" {
		t.Fatalf("%s (%s): FailoverMechanism is unset, want a real automatic-promotion claim — see %s", spec.ID, spec.Rule, spec.Source)
	}
}

// TestElastiCacheFailover_Disabled_002 is FAILOVER-ELASTICACHE-002: the negative case
// — a replication group with automatic failover disabled (or omitted, per its own
// documented Terraform default of false) must never claim automatic promotion it
// doesn't have.
func TestElastiCacheFailover_Disabled_002(t *testing.T) {
	spec := harness.Verify(t, harness.Spec{
		ID:            "FAILOVER-ELASTICACHE-002",
		Rule:          "An ElastiCache replication group without automatic failover enabled has no automatic promotion of a replica on primary failure.",
		Source:        "https://docs.aws.amazon.com/AmazonElastiCache/latest/red-ug/AutoFailover.html",
		Scenario:      "An aws_elasticache_replication_group declares automatic_failover_enabled = false.",
		Configuration: "resource \"aws_route53_record\" \"entry\" {}\nresource \"aws_elasticache_replication_group\" \"payments\" { automatic_failover_enabled = false }",
		Request:       "Ingest this configuration and read the resulting cache node's FailoverMechanism capability field.",
		Expected:      "FailoverMechanism is the explicit \"none\" sentinel, never the enabled-case wording.",
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
	node := findNodeByType(t, result.IR.Nodes, core.NodeTypeCache)
	if node.Capability == nil || node.Capability.FailoverMechanism == nil {
		t.Fatalf("%s (%s): FailoverMechanism is unset, want the explicit none sentinel — see %s", spec.ID, spec.Rule, spec.Source)
	}
	if *node.Capability.FailoverMechanism != "none" {
		t.Fatalf("%s (%s): FailoverMechanism = %q, want the \"none\" sentinel", spec.ID, spec.Rule, *node.Capability.FailoverMechanism)
	}
}
