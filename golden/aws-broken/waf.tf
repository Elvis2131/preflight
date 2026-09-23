# DEFECT 6 (compliance, silent): the web ACL exists and looks correct in isolation, but
# it is never associated with the ALB - the association resource is absent. Nothing is
# actually inspected. This is the defect most likely to pass a human eyeball review, and
# it is exactly the kind of finding that requires the graph rather than a resource list:
# the web_acl node has no edge to the load_balancer node.
# The SQLi managed rule group has also been dropped.
#
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

