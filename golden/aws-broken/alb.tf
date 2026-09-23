# GOLDEN NODE TYPE 3/8: load_balancer (ALB)
# Spans all three AZs. This is the node whose subnet spread decides whether AZ loss
# (§14 scenario 1) is survivable at the edge.

resource "aws_lb" "payments" {
  name                       = "payments-alb"
  internal                   = false
  load_balancer_type         = "application"
  security_groups            = [aws_security_group.alb.id]
  subnets                    = [aws_subnet.public_a.id, aws_subnet.public_b.id, aws_subnet.public_c.id]
  drop_invalid_header_fields = true
  enable_deletion_protection = true

  tags = {
    Name       = "payments-alb"
    Tier       = "tier1"
    Compliance = "PCI"
  }
}

resource "aws_lb_target_group" "payments" {
  name        = "payments-tg"
  port        = 8080
  protocol    = "HTTP"
  target_type = "ip"
  vpc_id      = aws_vpc.payments.id

  health_check {
    enabled             = true
    path                = "/healthz"
    protocol            = "HTTP"
    healthy_threshold   = 2
    unhealthy_threshold = 2
    interval            = 10
    timeout             = 5
    matcher             = "200"
  }

  tags = {
    Name = "payments-tg"
  }
}

resource "aws_lb_listener" "https" {
  load_balancer_arn = aws_lb.payments.arn
  port              = 443
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"
  certificate_arn   = aws_acm_certificate.payments.arn

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.payments.arn
  }
}

# Redirect rather than serve plaintext - PCI, and a compliance rule the engine can assert.
resource "aws_lb_listener" "http_redirect" {
  load_balancer_arn = aws_lb.payments.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type = "redirect"

    redirect {
      port        = "443"
      protocol    = "HTTPS"
      status_code = "HTTP_301"
    }
  }
}

resource "aws_acm_certificate" "payments" {
  domain_name       = "api.payments.example.com"
  validation_method = "DNS"

  tags = {
    Name = "payments-api-cert"
  }
}
