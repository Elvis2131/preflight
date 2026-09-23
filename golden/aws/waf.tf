# GOLDEN NODE TYPE 2/8: waf (WAFv2)
# Associated to the ALB - an unassociated web ACL is a compliance finding, and the
# association edge is what makes that detectable rather than assumed.

resource "aws_wafv2_web_acl" "payments" {
  name  = "payments-api-waf"
  scope = "REGIONAL"

  default_action {
    allow {}
  }

  rule {
    name     = "AWSManagedRulesCommonRuleSet"
    priority = 1

    override_action {
      none {}
    }

    statement {
      managed_rule_group_statement {
        name        = "AWSManagedRulesCommonRuleSet"
        vendor_name = "AWS"
      }
    }

    visibility_config {
      cloudwatch_metrics_enabled = true
      metric_name                = "payments-common-rules"
      sampled_requests_enabled   = true
    }
  }

  rule {
    name     = "AWSManagedRulesSQLiRuleSet"
    priority = 2

    override_action {
      none {}
    }

    statement {
      managed_rule_group_statement {
        name        = "AWSManagedRulesSQLiRuleSet"
        vendor_name = "AWS"
      }
    }

    visibility_config {
      cloudwatch_metrics_enabled = true
      metric_name                = "payments-sqli-rules"
      sampled_requests_enabled   = true
    }
  }

  visibility_config {
    cloudwatch_metrics_enabled = true
    metric_name                = "payments-api-waf"
    sampled_requests_enabled   = true
  }

  tags = {
    Name       = "payments-api-waf"
    Compliance = "PCI"
  }
}

resource "aws_wafv2_web_acl_association" "payments" {
  resource_arn = aws_lb.payments.arn
  web_acl_arn  = aws_wafv2_web_acl.payments.arn
}
