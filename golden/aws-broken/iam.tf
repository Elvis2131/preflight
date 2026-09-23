# DEFECT 7 (identity, tier1): the application role carries Action "*" on Resource "*".
# The identity/credential-failure golden scenario (§14) becomes unbounded - a compromised
# pod credential reaches every resource in the account, and the queue/key edges the graph
# previously encoded are gone, replaced by an implicit edge to everything.
#
# GOLDEN NODE TYPE 8/8: identity (IAM)
# Identity/credential failure is one of the six golden scenarios (§14), so roles are
# modelled as real nodes with real trust relationships - not as an afterthought. The
# application role is least-privilege and resource-scoped; the broken variant replaces
# it with a wildcard so the compliance engine has a genuine regression to catch.

resource "aws_iam_role" "eks_cluster" {
  name = "payments-eks-cluster-role"

  assume_role_policy = <<-EOT
    {
      "Version": "2012-10-17",
      "Statement": [
        {
          "Effect": "Allow",
          "Principal": { "Service": "eks.amazonaws.com" },
          "Action": "sts:AssumeRole"
        }
      ]
    }
  EOT

  tags = {
    Name = "payments-eks-cluster-role"
  }
}

resource "aws_iam_role_policy_attachment" "eks_cluster_policy" {
  role       = aws_iam_role.eks_cluster.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSClusterPolicy"
}

resource "aws_iam_role" "eks_node" {
  name = "payments-eks-node-role"

  assume_role_policy = <<-EOT
    {
      "Version": "2012-10-17",
      "Statement": [
        {
          "Effect": "Allow",
          "Principal": { "Service": "ec2.amazonaws.com" },
          "Action": "sts:AssumeRole"
        }
      ]
    }
  EOT

  tags = {
    Name = "payments-eks-node-role"
  }
}

resource "aws_iam_role_policy_attachment" "eks_node_worker" {
  role       = aws_iam_role.eks_node.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy"
}

resource "aws_iam_role_policy_attachment" "eks_node_cni" {
  role       = aws_iam_role.eks_node.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy"
}

resource "aws_iam_role_policy_attachment" "eks_node_ecr" {
  role       = aws_iam_role.eks_node.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
}

# --- Application role (IRSA) ----------------------------------------------------------
# Least-privilege and resource-scoped: the queue it may use and the key it may decrypt
# with are named explicitly. No wildcards.

resource "aws_iam_role" "payments_app" {
  name = "payments-app-role"

  assume_role_policy = <<-EOT
    {
      "Version": "2012-10-17",
      "Statement": [
        {
          "Effect": "Allow",
          "Principal": { "Service": "pods.eks.amazonaws.com" },
          "Action": ["sts:AssumeRole", "sts:TagSession"]
        }
      ]
    }
  EOT

  tags = {
    Name       = "payments-app-role"
    Compliance = "PCI"
  }
}

resource "aws_iam_policy" "payments_app" {
  name        = "payments-app-policy"
  description = "Access for the payments API workload."

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid      = "AppAccess"
        Effect   = "Allow"
        Action   = "*"
        Resource = "*"
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "payments_app" {
  role       = aws_iam_role.payments_app.name
  policy_arn = aws_iam_policy.payments_app.arn
}
