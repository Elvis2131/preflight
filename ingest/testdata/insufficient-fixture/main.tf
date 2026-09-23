resource "aws_db_instance" "payments" {
  engine         = "postgres"
  instance_class = "db.r6g.large"
  multi_az       = true
}

resource "aws_iam_role" "app" {
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "ec2.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}
