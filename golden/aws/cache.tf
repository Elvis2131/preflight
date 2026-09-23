# GOLDEN NODE TYPE 6/8: cache (ElastiCache)
# Replication group with automatic failover across AZs. A cache is not a stateful node
# for the Minimum Viable Graph check, but its loss is one of the six golden scenarios,
# so it needs a real failover posture to reason about.

resource "aws_elasticache_subnet_group" "payments" {
  name       = "payments-cache-subnets"
  subnet_ids = [aws_subnet.data_a.id, aws_subnet.data_b.id, aws_subnet.data_c.id]
}

resource "aws_elasticache_replication_group" "payments" {
  replication_group_id = "payments-cache"
  description          = "Session and idempotency-key cache for the payments API."

  engine         = "redis"
  engine_version = "7.1"
  node_type      = "cache.r6g.large"
  port           = 6379

  # Three nodes across three AZs, with automatic failover.
  num_cache_clusters         = 3
  automatic_failover_enabled = true
  multi_az_enabled           = true

  subnet_group_name  = aws_elasticache_subnet_group.payments.name
  security_group_ids = [aws_security_group.cache.id]

  at_rest_encryption_enabled = true
  transit_encryption_enabled = true
  kms_key_id                 = aws_kms_key.payments.arn

  snapshot_retention_limit = 7
  snapshot_window          = "01:00-02:00"
  maintenance_window       = "sun:04:30-sun:05:30"

  tags = {
    Name       = "payments-cache"
    Tier       = "tier1"
    Compliance = "PCI"
  }
}
