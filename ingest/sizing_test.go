package ingest_test

// PC-115: proves Sizing is populated from real Terraform attributes on the golden AWS
// bundle — never a fixture invented for this test, since the whole point is that this
// is real, already-existing infrastructure declaration, not a synthetic shape.

import (
	"path/filepath"
	"testing"

	"preflight/ingest"
)

func TestSizing_PopulatedFromRealGoldenAttributes(t *testing.T) {
	reg := loadRegistry(t)
	result, err := ingest.Ingest(filepath.Join("..", "golden", "aws"), reg, 1)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if result.Insufficient != nil {
		t.Fatalf("got insufficient_model: %+v", result.Insufficient)
	}

	foundRDS, foundCache, foundALB, foundEKS := false, false, false, false
	for _, n := range result.IR.Nodes {
		switch n.ID {
		case "aws_db_instance.payments":
			foundRDS = true
			if n.Sizing == nil {
				t.Fatal("aws_db_instance.payments: Sizing is nil, want populated from real attributes")
			}
			if n.Sizing.InstanceClass == nil || *n.Sizing.InstanceClass != "db.r6g.xlarge" {
				t.Errorf("InstanceClass = %v, want db.r6g.xlarge", n.Sizing.InstanceClass)
			}
			if n.Sizing.AllocatedStorageGB == nil || *n.Sizing.AllocatedStorageGB != 200 {
				t.Errorf("AllocatedStorageGB = %v, want 200", n.Sizing.AllocatedStorageGB)
			}
			if n.Sizing.StorageType == nil || *n.Sizing.StorageType != "gp3" {
				t.Errorf("StorageType = %v, want gp3", n.Sizing.StorageType)
			}
		case "aws_elasticache_replication_group.payments":
			foundCache = true
			if n.Sizing == nil {
				t.Fatal("elasticache: Sizing is nil, want populated")
			}
			if n.Sizing.CacheNodeType == nil || *n.Sizing.CacheNodeType != "cache.r6g.large" {
				t.Errorf("CacheNodeType = %v, want cache.r6g.large", n.Sizing.CacheNodeType)
			}
			if n.Sizing.Count == nil || *n.Sizing.Count != 3 {
				t.Errorf("Count = %v, want 3 (num_cache_clusters)", n.Sizing.Count)
			}
		case "aws_lb.payments":
			foundALB = true
			if n.Sizing == nil || n.Sizing.LoadBalancerType == nil {
				t.Fatalf("aws_lb: Sizing/LoadBalancerType is nil, want application")
			}
			if *n.Sizing.LoadBalancerType != "application" {
				t.Errorf("LoadBalancerType = %v, want application", *n.Sizing.LoadBalancerType)
			}
		case "aws_eks_cluster.payments":
			foundEKS = true
			if n.Sizing == nil {
				t.Fatal("eks cluster: Sizing is nil, want populated from the companion aws_eks_node_group")
			}
			if n.Sizing.InstanceType == nil || *n.Sizing.InstanceType != "m6i.large" {
				t.Errorf("InstanceType = %v, want m6i.large (from aws_eks_node_group.instance_types[0])", n.Sizing.InstanceType)
			}
			if n.Sizing.Count == nil || *n.Sizing.Count != 3 {
				t.Errorf("Count = %v, want 3 (scaling_config.desired_size)", n.Sizing.Count)
			}
		}
	}
	if !foundRDS || !foundCache || !foundALB || !foundEKS {
		t.Fatalf("missing expected golden nodes: rds=%v cache=%v alb=%v eks=%v", foundRDS, foundCache, foundALB, foundEKS)
	}
}

// TestSizing_AbsentMeansNil proves the acceptance criterion directly: a node with no
// matching real attribute gets a nil Sizing, never a synthesized default.
func TestSizing_AbsentMeansNil(t *testing.T) {
	reg := loadRegistry(t)
	result, err := ingest.Ingest(filepath.Join("..", "golden", "aws"), reg, 1)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	for _, n := range result.IR.Nodes {
		if n.ID == "aws_route53_record.api" && n.Sizing != nil {
			t.Errorf("aws_route53_record has no sizing-relevant attributes at all; want nil Sizing, got %+v", n.Sizing)
		}
	}
}
