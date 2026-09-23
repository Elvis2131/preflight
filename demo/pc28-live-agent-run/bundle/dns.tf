# DEFECT 8 (recovery): no health check, and the alias does not evaluate target health.
# DNS will keep answering with the ALB regardless of its state, so the region-loss golden
# scenario has no detection or failover path at the edge - RTO 60s is unachievable.
#
# GOLDEN NODE TYPE 1/8: dns (Route 53)
# The entry point for the Minimum Viable Graph check (§15): a graph with zero entry
# points is `insufficient_model`, not a pile of findings.

resource "aws_route53_zone" "payments" {
  name    = var.dns_zone_name
  comment = "Public zone for the payments API."

  tags = {
    Service = "payments-api"
  }
}

# Alias to the ALB. This is the edge that makes dns -> load_balancer a real graph edge
# rather than two disconnected nodes.
resource "aws_route53_record" "api" {
  zone_id = aws_route53_zone.payments.zone_id
  name    = "api.payments.example.com"
  type    = "A"

  alias {
    name                   = aws_lb.payments.dns_name
    zone_id                = aws_lb.payments.zone_id
    evaluate_target_health = false
  }
}
