# DEFECT 5 (SPOF + PCI, tier1): the workload runs as a single replica in a single AZ,
# the control plane is publicly reachable, and envelope encryption of Kubernetes secrets
# is off.
#   - node group in private_a only, desired_size 1 -> AZ-a loss is a total outage, and
#     there is no rolling-update headroom (max_unavailable 1 of 1).
#   - endpoint_public_access = true -> PCI finding on a cardholder-data workload.
# Expected: tier-1 SPOF at the node group plus a public-endpoint compliance finding.
#
# GOLDEN NODE TYPE 4/8: container_workload (EKS)
# Node group spans three AZs. Private endpoint access only - a publicly reachable
# control plane is a PCI finding.

resource "aws_eks_cluster" "payments" {
  name     = "payments-eks"
  role_arn = aws_iam_role.eks_cluster.arn
  version  = "1.31"

  vpc_config {
    subnet_ids              = [aws_subnet.private_a.id, aws_subnet.private_b.id]
    endpoint_private_access = true
    endpoint_public_access  = true
    security_group_ids      = [aws_security_group.workload.id]
  }

  depends_on = [
    aws_iam_role_policy_attachment.eks_cluster_policy
  ]

  tags = {
    Name       = "payments-eks"
    Tier       = "tier1"
    Compliance = "PCI"
  }
}

resource "aws_eks_node_group" "payments" {
  cluster_name    = aws_eks_cluster.payments.name
  node_group_name = "payments-workers"
  node_role_arn   = aws_iam_role.eks_node.arn
  subnet_ids      = [aws_subnet.private_a.id]
  instance_types  = ["m6i.large"]

  scaling_config {
    desired_size = 1
    min_size     = 1
    max_size     = 3
  }

  update_config {
    max_unavailable = 1
  }

  depends_on = [
    aws_iam_role_policy_attachment.eks_node_worker,
    aws_iam_role_policy_attachment.eks_node_cni,
    aws_iam_role_policy_attachment.eks_node_ecr
  ]

  tags = {
    Name = "payments-workers"
  }
}
