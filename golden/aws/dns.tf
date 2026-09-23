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

resource "aws_route53_health_check" "payments_primary" {
  fqdn              = "api.payments.example.com"
  port              = 443
  type              = "HTTPS"
  resource_path     = "/healthz"
  failure_threshold = 3
  request_interval  = 10

  tags = {
    Name = "payments-primary-health"
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
    evaluate_target_health = true
  }
}
