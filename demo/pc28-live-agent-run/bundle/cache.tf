# DEFECT 3 (SPOF + PCI): one cache node, no failover, no encryption in transit or at
# rest. Idempotency keys for payment requests live here, so losing it is not a
# performance event - it risks duplicate settlement. Expected: SPOF finding plus a
# transit-encryption compliance finding.
#
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

  num_cache_clusters         = 1
  automatic_failover_enabled = false
  multi_az_enabled           = false

  subnet_group_name  = aws_elasticache_subnet_group.payments.name
  security_group_ids = [aws_security_group.cache.id]

  at_rest_encryption_enabled = false
  transit_encryption_enabled = false

  snapshot_retention_limit = 0
  snapshot_window          = "01:00-02:00"
  maintenance_window       = "sun:04:30-sun:05:30"

  tags = {
    Name       = "payments-cache"
    Tier       = "tier1"
    Compliance = "PCI"
  }
}
