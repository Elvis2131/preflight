# GOLDEN NODE TYPE 4/8: container_workload (EKS)
# Node group spans three AZs. Private endpoint access only - a publicly reachable
# control plane is a PCI finding.

resource "aws_eks_cluster" "payments" {
  name     = "payments-eks"
  role_arn = aws_iam_role.eks_cluster.arn
  version  = "1.31"

  vpc_config {
    subnet_ids              = [aws_subnet.private_a.id, aws_subnet.private_b.id, aws_subnet.private_c.id]
    endpoint_private_access = true
    endpoint_public_access  = false
    security_group_ids      = [aws_security_group.workload.id]
  }

  encryption_config {
    provider {
      key_arn = aws_kms_key.payments.arn
    }

    resources = ["secrets"]
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
  subnet_ids      = [aws_subnet.private_a.id, aws_subnet.private_b.id, aws_subnet.private_c.id]
  instance_types  = ["m6i.large"]

  # Three desired across three AZs: losing one AZ leaves two healthy replicas.
  scaling_config {
    desired_size = 3
    min_size     = 3
    max_size     = 9
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
