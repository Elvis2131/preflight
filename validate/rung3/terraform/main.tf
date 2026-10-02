# PC-25 — Rung 3 experiment bundle: two web instances in two AZs behind an internet-facing ALB.
# The runner stops the instance in AZ "a" with a direct EC2 StopInstances call (the same API call AWS
# FIS's aws:ec2:stop-instances action makes; FIS itself is denied in this account by an organisation
# service control policy, so it is not used). Everything is ephemeral: the runner applies, injects the
# fault, captures, and destroys in one run (cmd/runnerd/internal/rung3).
#
# Deliberately small and cheap: no NAT gateway, no public IPs on the instances (they never need the
# internet), two t3.micro and one ALB — roughly USD 0.05 per hour while it exists.
#
# This same bundle is also ingested by Preflight itself (validate/rung3 Predict) BEFORE the run, so
# the prediction and the observation are about one architecture, and the prediction is committed
# before the experiment is.

terraform {
  required_version = ">= 1.5"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

variable "region" {
  type    = string
  default = "eu-north-1"
}

variable "run_id" {
  type    = string
  default = "manual"
}

variable "az_a" {
  type    = string
  default = "eu-north-1a"
}

variable "az_b" {
  type    = string
  default = "eu-north-1b"
}

provider "aws" {
  region = var.region
  default_tags {
    tags = {
      preflight-experiment = "rung3"
      preflight-run        = var.run_id
    }
  }
}

data "aws_ssm_parameter" "al2023" {
  name = "/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-x86_64"
}

resource "aws_vpc" "main" {
  cidr_block           = "10.42.0.0/16"
  enable_dns_support   = true
  enable_dns_hostnames = true
}

resource "aws_subnet" "a" {
  vpc_id                  = aws_vpc.main.id
  cidr_block              = "10.42.1.0/24"
  availability_zone       = var.az_a
  map_public_ip_on_launch = false
}

resource "aws_subnet" "b" {
  vpc_id                  = aws_vpc.main.id
  cidr_block              = "10.42.2.0/24"
  availability_zone       = var.az_b
  map_public_ip_on_launch = false
}

resource "aws_internet_gateway" "main" {
  vpc_id = aws_vpc.main.id
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.main.id
  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.main.id
  }
}

resource "aws_route_table_association" "a" {
  subnet_id      = aws_subnet.a.id
  route_table_id = aws_route_table.public.id
}

resource "aws_route_table_association" "b" {
  subnet_id      = aws_subnet.b.id
  route_table_id = aws_route_table.public.id
}

resource "aws_security_group" "alb" {
  name   = "preflight-rung3-alb-${var.run_id}"
  vpc_id = aws_vpc.main.id
  ingress {
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_security_group" "web" {
  name   = "preflight-rung3-web-${var.run_id}"
  vpc_id = aws_vpc.main.id
  ingress {
    from_port       = 80
    to_port         = 80
    protocol        = "tcp"
    security_groups = [aws_security_group.alb.id]
  }
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

# --- content origin: each web instance fetches its page from S3 at boot, over a free S3 gateway
# endpoint (no NAT, no internet). This is a real dependency, not decoration: if the fetch fails the
# instance never serves, never becomes healthy, and the run aborts before any fault is injected.
# It is also what makes this a model Preflight can assess at all: a stateless tier alone is below the
# Minimum Viable Graph (CLAUDE.md §15) and /assess answers insufficient_model.

resource "aws_s3_bucket" "content" {
  bucket_prefix = "pf-rung3-"
  force_destroy = true
}

resource "aws_s3_object" "page_a" {
  bucket       = aws_s3_bucket.content.id
  key          = "web-a.html"
  content      = "web-a"
  content_type = "text/html"
}

resource "aws_s3_object" "page_b" {
  bucket       = aws_s3_bucket.content.id
  key          = "web-b.html"
  content      = "web-b"
  content_type = "text/html"
}

resource "aws_vpc_endpoint" "s3" {
  vpc_id            = aws_vpc.main.id
  service_name      = "com.amazonaws.${var.region}.s3"
  vpc_endpoint_type = "Gateway"
  route_table_ids   = [aws_route_table.public.id]
}

resource "aws_iam_role" "web" {
  name = "preflight-rung3-web-${var.run_id}"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "ec2.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role_policy" "web_read_content" {
  name = "read-content"
  role = aws_iam_role.web.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["s3:GetObject"]
      Resource = "${aws_s3_bucket.content.arn}/*"
    }]
  })
}

resource "aws_iam_instance_profile" "web" {
  name = "preflight-rung3-web-${var.run_id}"
  role = aws_iam_role.web.name
}

resource "aws_instance" "web_a" {
  ami                    = data.aws_ssm_parameter.al2023.value
  instance_type          = "t3.micro"
  subnet_id              = aws_subnet.a.id
  vpc_security_group_ids = [aws_security_group.web.id]
  metadata_options {
    http_tokens = "required"
  }
  iam_instance_profile = aws_iam_instance_profile.web.name
  depends_on           = [aws_vpc_endpoint.s3, aws_s3_object.page_a]
  user_data            = <<-EOT
    #!/bin/bash
    mkdir -p /srv/www
    for i in $(seq 1 30); do
      aws s3 cp s3://${aws_s3_bucket.content.bucket}/web-a.html /srv/www/index.html --region ${var.region} && break
      sleep 5
    done
    cd /srv/www
    nohup python3 -m http.server 80 > /var/log/web.log 2>&1 &
  EOT
  tags = {
    Name = "preflight-rung3-web-a"
  }
}

resource "aws_instance" "web_b" {
  ami                    = data.aws_ssm_parameter.al2023.value
  instance_type          = "t3.micro"
  subnet_id              = aws_subnet.b.id
  vpc_security_group_ids = [aws_security_group.web.id]
  metadata_options {
    http_tokens = "required"
  }
  iam_instance_profile = aws_iam_instance_profile.web.name
  depends_on           = [aws_vpc_endpoint.s3, aws_s3_object.page_b]
  user_data            = <<-EOT
    #!/bin/bash
    mkdir -p /srv/www
    for i in $(seq 1 30); do
      aws s3 cp s3://${aws_s3_bucket.content.bucket}/web-b.html /srv/www/index.html --region ${var.region} && break
      sleep 5
    done
    cd /srv/www
    nohup python3 -m http.server 80 > /var/log/web.log 2>&1 &
  EOT
  tags = {
    Name = "preflight-rung3-web-b"
  }
}

resource "aws_lb" "main" {
  name               = "pf-rung3-${var.run_id}"
  load_balancer_type = "application"
  internal           = false
  subnets            = [aws_subnet.a.id, aws_subnet.b.id]
  security_groups    = [aws_security_group.alb.id]
}

# Health-check settings are declared (not left to defaults) so the prediction can be derived from
# them: unhealthy after 2 consecutive failures, one check every 10 seconds.
resource "aws_lb_target_group" "web" {
  name     = "pf-rung3-${var.run_id}"
  port     = 80
  protocol = "HTTP"
  vpc_id   = aws_vpc.main.id
  health_check {
    path                = "/"
    interval            = 10
    timeout             = 5
    healthy_threshold   = 2
    unhealthy_threshold = 2
    matcher             = "200"
  }
  deregistration_delay = 5
}

resource "aws_lb_target_group_attachment" "a" {
  target_group_arn = aws_lb_target_group.web.arn
  target_id        = aws_instance.web_a.id
  port             = 80
}

resource "aws_lb_target_group_attachment" "b" {
  target_group_arn = aws_lb_target_group.web.arn
  target_id        = aws_instance.web_b.id
  port             = 80
}

resource "aws_lb_listener" "http" {
  load_balancer_arn = aws_lb.main.arn
  port              = 80
  protocol          = "HTTP"
  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.web.arn
  }
}

output "alb_dns_name" {
  value = aws_lb.main.dns_name
}

output "target_group_arn" {
  value = aws_lb_target_group.web.arn
}

output "instance_a_id" {
  value = aws_instance.web_a.id
}

output "instance_b_id" {
  value = aws_instance.web_b.id
}
