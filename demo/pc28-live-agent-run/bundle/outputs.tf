output "api_endpoint" {
  description = "Public entry point for the payments API."
  value       = aws_route53_record.api.name
}

output "load_balancer_arn" {
  description = "ALB ARN - the WAF association target."
  value       = aws_lb.payments.arn
}

output "cluster_name" {
  description = "EKS cluster hosting the payments workload."
  value       = aws_eks_cluster.payments.name
}

output "database_endpoint" {
  description = "Payments database writer endpoint."
  value       = aws_db_instance.payments.endpoint
}

output "cache_endpoint" {
  description = "ElastiCache primary endpoint."
  value       = aws_elasticache_replication_group.payments.primary_endpoint_address
}

output "settlement_queue_url" {
  description = "Settlement queue URL."
  value       = aws_sqs_queue.settlement.url
}
