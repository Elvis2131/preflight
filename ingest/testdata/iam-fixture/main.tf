# PC-133's own synthetic fixture: golden/aws has no aws_iam_role_policy,
# aws_iam_policy, aws_iam_role_policy_attachment, or aws_s3_bucket_policy at all
# (only aws_iam_role's own assume_role_policy, already covered by a real golden-bundle
# hand-verification in ingest/iam_test.go) — this is the only real ingest coverage for
# the other three shapes. Policy bodies are real JSON heredoc strings (golden/aws/
# iam.tf's own real shape — never jsonencode(), a function call ingest's own static-
# literal parser (CLAUDE.md §15) cannot capture at all). Exercises wildcards (never
# expanded), a Condition block (preserved verbatim), and both the bare-string and
# array shapes of Action/Resource.

resource "aws_route53_record" "entry" {}

resource "aws_db_instance" "db" {}

resource "aws_s3_bucket" "artifacts" {}

resource "aws_iam_role" "app" {
  assume_role_policy = <<-EOT
    {
      "Version": "2012-10-17",
      "Statement": [
        {
          "Effect": "Allow",
          "Principal": { "Service": "ecs-tasks.amazonaws.com" },
          "Action": "sts:AssumeRole"
        }
      ]
    }
  EOT
}

# Inline identity policy — a real Action array, a wildcard Resource, and a real
# Condition block, all preserved verbatim.
resource "aws_iam_role_policy" "app_inline" {
  name = "app-inline-policy"
  role = aws_iam_role.app.id
  policy = <<-EOT
    {
      "Version": "2012-10-17",
      "Statement": [
        {
          "Sid": "ReadSecrets",
          "Effect": "Allow",
          "Action": ["secretsmanager:GetSecretValue", "secretsmanager:DescribeSecret"],
          "Resource": "arn:aws:secretsmanager:eu-west-1:123456789012:secret:payments/*",
          "Condition": {
            "StringEquals": { "aws:RequestedRegion": "eu-west-1" }
          }
        }
      ]
    }
  EOT
}

# A standalone, reusable managed policy document.
resource "aws_iam_policy" "read_artifacts" {
  name = "read-artifacts"
  policy = <<-EOT
    {
      "Version": "2012-10-17",
      "Statement": [
        {
          "Effect": "Allow",
          "Action": "s3:GetObject",
          "Resource": "arn:aws:s3:::payments-artifacts/*"
        }
      ]
    }
  EOT
}

resource "aws_iam_role_policy_attachment" "app_read_artifacts" {
  role       = aws_iam_role.app.name
  policy_arn = aws_iam_policy.read_artifacts.arn
}

# A resource-based (bucket) policy — attached to the bucket, not any identity.
resource "aws_s3_bucket_policy" "artifacts" {
  bucket = aws_s3_bucket.artifacts.id
  policy = <<-EOT
    {
      "Version": "2012-10-17",
      "Statement": [
        {
          "Effect": "Deny",
          "Principal": "*",
          "Action": "s3:*",
          "Resource": ["arn:aws:s3:::payments-artifacts", "arn:aws:s3:::payments-artifacts/*"],
          "Condition": {
            "Bool": { "aws:SecureTransport": "false" }
          }
        }
      ]
    }
  EOT
}
