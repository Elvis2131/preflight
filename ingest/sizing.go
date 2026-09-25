// This file is PC-115's ingest-side half: real sizing facts read from a resource's own
// parsed Terraform attributes into core.Sizing — never a default, never guessed.
// Direct, same-resource attributes only (aws_db_instance's own instance_class, etc.);
// the one cross-resource case (an EKS node group's own instance/count sizing, declared
// on a SEPARATE aws_eks_node_group resource from the aws_eks_cluster node it sizes) is
// handled separately by mergeEKSNodeGroupSizing, the same "merge onto the already-
// built owning node" shape mergeSecurityGroupRules/mergeNetworkACLRules already use.
package ingest

import "preflight/core"

func attrString(attrs map[string]any, key string) *string {
	if v, ok := attrs[key].(string); ok && v != "" {
		return &v
	}
	return nil
}

func attrInt(attrs map[string]any, key string) *int {
	switch v := attrs[key].(type) {
	case float64:
		n := int(v)
		return &n
	case int:
		return &v
	default:
		return nil
	}
}

// buildSizing reads whichever of this resource's own real, literal Terraform
// attributes correspond to core.Sizing's fields — attribute names verified against
// the golden bundle's own real Terraform (golden/aws/rds.tf, cache.tf, alb.tf).
// Fields with no matching attribute simply stay nil (cost_unknown for that dimension),
// never defaulted.
func buildSizing(attrs map[string]any) *core.Sizing {
	s := core.Sizing{
		InstanceClass:      attrString(attrs, "instance_class"),
		AllocatedStorageGB: attrInt(attrs, "allocated_storage"),
		StorageType:        attrString(attrs, "storage_type"),
		CacheNodeType:      attrString(attrs, "node_type"),
		Count:              attrInt(attrs, "num_cache_clusters"),
		LoadBalancerType:   attrString(attrs, "load_balancer_type"),
	}
	if s == (core.Sizing{}) {
		return nil
	}
	return &s
}

// mergeEKSNodeGroupSizing merges an aws_eks_node_group's own instance_types[0] and
// scaling_config.desired_size onto the owning aws_eks_cluster node's Sizing — real
// sizing data for a container_workload node lives on this separate Terraform resource,
// never on the cluster resource itself, so this cannot be read directly in buildNode
// (called before this companion resource has even been examined) — the same
// "post-process onto the already-built node slice, by ID" shape PC-112/113's own
// merge functions use for security_group_rules/nacl_rules.
func mergeEKSNodeGroupSizing(nodes []core.Node, parsed []ParsedResource) {
	for _, r := range parsed {
		if r.Type != "aws_eks_node_group" {
			continue
		}
		clusterRefs, ok := r.AttributeReferences["cluster_name"]
		if !ok || len(clusterRefs) != 1 {
			continue
		}
		clusterKey := clusterRefs[0].Key()

		var instanceType *string
		if types, ok := r.Attributes["instance_types"].([]any); ok && len(types) > 0 {
			if s, ok := types[0].(string); ok && s != "" {
				instanceType = &s
			}
		}
		count := attrInt(r.Attributes, "scaling_config.desired_size")

		if instanceType == nil && count == nil {
			continue
		}
		for i := range nodes {
			if nodes[i].ID != clusterKey {
				continue
			}
			sizing := nodes[i].Sizing
			if sizing == nil {
				sizing = &core.Sizing{}
			}
			if instanceType != nil {
				sizing.InstanceType = instanceType
			}
			if count != nil {
				sizing.Count = count
			}
			nodes[i].Sizing = sizing
		}
	}
}
