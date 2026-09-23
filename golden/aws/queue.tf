# GOLDEN NODE TYPE 7/8: queue (SQS)
# Settlement queue with a dead-letter queue. The DLQ is what turns "queue failure"
# (§14 scenario 3) into a bounded, analysable outcome rather than silent message loss.

resource "aws_sqs_queue" "settlement_dlq" {
  name                      = "payments-settlement-dlq"
  message_retention_seconds = 1209600
  kms_master_key_id         = aws_kms_key.payments.id

  tags = {
    Name       = "payments-settlement-dlq"
    Compliance = "PCI"
  }
}

resource "aws_sqs_queue" "settlement" {
  name                       = "payments-settlement"
  visibility_timeout_seconds = 60
  message_retention_seconds  = 345600
  receive_wait_time_seconds  = 20

  kms_master_key_id                 = aws_kms_key.payments.id
  kms_data_key_reuse_period_seconds = 300

  # NOTE (§15 parser boundary): this is a function call, not a bare static reference.
  # It stays as real Terraform idiom on purpose - there is no way to express a redrive
  # policy without encoding JSON, and hardcoding the DLQ ARN would erase the
  # queue -> dead-letter-queue edge the failure model needs. This is a deliberate test
  # case for PC-12: extract `aws_sqs_queue.settlement_dlq.arn` from inside jsonencode(),
  # or surface the reference as `unresolved` - never drop the edge silently.
  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.settlement_dlq.arn
    maxReceiveCount     = 5
  })

  tags = {
    Name       = "payments-settlement"
    Tier       = "tier1"
    Compliance = "PCI"
  }
}
