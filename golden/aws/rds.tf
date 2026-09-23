# GOLDEN NODE TYPE 5/8: managed_database (RDS)
# Multi-AZ with a synchronous standby. This is what makes the workload's RPO 0 claim
# (workload.yaml) structurally defensible - and it is exactly what the broken variant
# removes, so the simulator has a real RPO regression to detect.

resource "aws_db_subnet_group" "payments" {
  name       = "payments-db-subnets"
  subnet_ids = [aws_subnet.data_a.id, aws_subnet.data_b.id, aws_subnet.data_c.id]

  tags = {
    Name = "payments-db-subnets"
  }
}

resource "aws_db_parameter_group" "payments" {
  name   = "payments-pg16"
  family = "postgres16"

  parameter {
    name  = "rds.force_ssl"
    value = "1"
  }

  tags = {
    Name       = "payments-pg16"
    Compliance = "PCI"
  }
}

resource "aws_db_instance" "payments" {
  identifier     = "payments-db"
  engine         = "postgres"
  engine_version = "16.4"
  instance_class = "db.r6g.xlarge"

  allocated_storage     = 200
  max_allocated_storage = 1000
  storage_type          = "gp3"
  storage_encrypted     = true
  kms_key_id            = aws_kms_key.payments.arn

  db_name  = "payments"
  username = var.db_username

  # Synchronous standby in a second AZ -> RPO 0, and an automatic failover path
  # that the RTO 60s target depends on.
  multi_az = true

  db_subnet_group_name   = aws_db_subnet_group.payments.name
  parameter_group_name   = aws_db_parameter_group.payments.name
  vpc_security_group_ids = [aws_security_group.database.id]

  backup_retention_period = 14
  backup_window           = "02:00-03:00"
  maintenance_window      = "sun:03:30-sun:04:30"
  copy_tags_to_snapshot   = true

  deletion_protection      = true
  delete_automated_backups = false
  skip_final_snapshot      = false
  final_snapshot_identifier = "payments-db-final"

  performance_insights_enabled          = true
  performance_insights_kms_key_id       = aws_kms_key.payments.arn
  enabled_cloudwatch_logs_exports       = ["postgresql", "upgrade"]
  auto_minor_version_upgrade            = true

  tags = {
    Name       = "payments-db"
    Tier       = "tier1"
    Compliance = "PCI"
  }
}
