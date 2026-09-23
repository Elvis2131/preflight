# Security groups and the customer-managed key. PCI (§14) is why the CMK exists rather
# than relying on AWS-managed keys: the compliance engine (PC-18) needs a key resource
# with a rotation property it can actually produce evidence against.

resource "aws_kms_key" "payments" {
  description             = "CMK for payments-api data at rest (RDS, ElastiCache, SQS)."
  enable_key_rotation     = true
  deletion_window_in_days = 30

  tags = {
    Name       = "payments-cmk"
    Compliance = "PCI"
  }
}

resource "aws_kms_alias" "payments" {
  name          = "alias/payments-api"
  target_key_id = aws_kms_key.payments.key_id
}

resource "aws_security_group" "alb" {
  name        = "payments-alb-sg"
  description = "Public ingress to the payments ALB. HTTPS only."
  vpc_id      = aws_vpc.payments.id

  ingress {
    description = "HTTPS from the internet"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    description     = "To the EKS workload"
    from_port       = 8080
    to_port         = 8080
    protocol        = "tcp"
    security_groups = [aws_security_group.workload.id]
  }

  tags = {
    Name = "payments-alb-sg"
  }
}

resource "aws_security_group" "workload" {
  name        = "payments-workload-sg"
  description = "EKS payments workload."
  vpc_id      = aws_vpc.payments.id

  egress {
    description = "Outbound to AWS APIs and the external payment rail"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "payments-workload-sg"
  }
}

resource "aws_security_group_rule" "workload_from_alb" {
  type                     = "ingress"
  description              = "Application traffic from the ALB"
  from_port                = 8080
  to_port                  = 8080
  protocol                 = "tcp"
  security_group_id        = aws_security_group.workload.id
  source_security_group_id = aws_security_group.alb.id
}

resource "aws_security_group_rule" "workload_to_database" {
  type                     = "egress"
  description              = "PostgreSQL to the payments database"
  from_port                = 5432
  to_port                  = 5432
  protocol                 = "tcp"
  security_group_id        = aws_security_group.workload.id
  source_security_group_id = aws_security_group.database.id
}

resource "aws_security_group_rule" "workload_to_cache" {
  type                     = "egress"
  description              = "Redis to the payments cache"
  from_port                = 6379
  to_port                  = 6379
  protocol                 = "tcp"
  security_group_id        = aws_security_group.workload.id
  source_security_group_id = aws_security_group.cache.id
}

resource "aws_security_group" "database" {
  name        = "payments-database-sg"
  description = "Payments RDS. Reachable only from the workload security group."
  vpc_id      = aws_vpc.payments.id

  ingress {
    description     = "PostgreSQL from the EKS workload"
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [aws_security_group.workload.id]
  }

  tags = {
    Name = "payments-database-sg"
  }
}

resource "aws_security_group" "cache" {
  name        = "payments-cache-sg"
  description = "Payments ElastiCache. Reachable only from the workload security group."
  vpc_id      = aws_vpc.payments.id

  ingress {
    description     = "Redis from the EKS workload"
    from_port       = 6379
    to_port         = 6379
    protocol        = "tcp"
    security_groups = [aws_security_group.workload.id]
  }

  tags = {
    Name = "payments-cache-sg"
  }
}
