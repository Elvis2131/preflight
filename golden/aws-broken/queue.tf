# DEFECT 4 (durability + PCI): the settlement queue has no dead-letter queue and no
# customer-managed encryption. A poison message is retried until retention expires and
# is then lost silently - the "queue failure" golden scenario becomes unbounded message
# loss rather than a bounded, inspectable backlog.
#
# GOLDEN NODE TYPE 7/8: queue (SQS)
# Settlement queue with a dead-letter queue. The DLQ is what turns "queue failure"
# (§14 scenario 3) into a bounded, analysable outcome rather than silent message loss.

resource "aws_sqs_queue" "settlement" {
  name                       = "payments-settlement"
  visibility_timeout_seconds = 60
  message_retention_seconds  = 345600
  receive_wait_time_seconds  = 20

  tags = {
    Name       = "payments-settlement"
    Tier       = "tier1"
    Compliance = "PCI"
  }
}
