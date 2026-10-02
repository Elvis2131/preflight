# PC-158 kitchen-sink bundle: one small synthetic design that uses every construct the golden bundles do not,
# so the value-level sweep reaches the ingest and engine paths golden never exercises. Not a model of a real
# architecture.

resource "aws_vpc" "v" { cidr_block = "10.0.0.0/16" }
resource "aws_internet_gateway" "igw" { vpc_id = aws_vpc.v.id }

resource "aws_subnet" "pub" {
  vpc_id                  = aws_vpc.v.id
  cidr_block              = "10.0.1.0/24"
  availability_zone       = "eu-west-1a"
  map_public_ip_on_launch = true
  tags = {
    Tier = "public"
  }
}
resource "aws_subnet" "app" {
  vpc_id            = aws_vpc.v.id
  cidr_block        = "10.0.2.0/24"
  availability_zone = "eu-west-1a"
}
resource "aws_subnet" "data" {
  vpc_id            = aws_vpc.v.id
  cidr_block        = "10.0.3.0/24"
  availability_zone = "eu-west-1b"
}

# routes: an inline table, a standalone aws_route, the main table, and the associations
resource "aws_route_table" "pub" {
  vpc_id = aws_vpc.v.id
  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.igw.id
  }
}
resource "aws_route_table_association" "pub" {
  subnet_id      = aws_subnet.pub.id
  route_table_id = aws_route_table.pub.id
}
resource "aws_eip" "nat" { domain = "vpc" }
resource "aws_nat_gateway" "nat" {
  allocation_id = aws_eip.nat.id
  subnet_id     = aws_subnet.pub.id
}
resource "aws_route_table" "app" { vpc_id = aws_vpc.v.id }
resource "aws_route" "app_default" {
  route_table_id         = aws_route_table.app.id
  destination_cidr_block = "0.0.0.0/0"
  nat_gateway_id         = aws_nat_gateway.nat.id
}
resource "aws_route_table_association" "app" {
  subnet_id      = aws_subnet.app.id
  route_table_id = aws_route_table.app.id
}
resource "aws_route_table" "main" {
  vpc_id = aws_vpc.v.id
  route {
    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.nat.id
  }
}
resource "aws_main_route_table_association" "main" {
  vpc_id         = aws_vpc.v.id
  route_table_id = aws_route_table.main.id
}

# NACLs: inline rules with inline association, a standalone rule, an association resource, the VPC default
resource "aws_network_acl" "app" {
  vpc_id     = aws_vpc.v.id
  subnet_ids = [aws_subnet.app.id]
  ingress {
    rule_no    = 100
    action     = "allow"
    protocol   = "tcp"
    cidr_block = "10.0.0.0/16"
    from_port  = 8080
    to_port    = 8080
  }
  egress {
    rule_no    = 100
    action     = "allow"
    protocol   = "-1"
    cidr_block = "0.0.0.0/0"
    from_port  = 0
    to_port    = 0
  }
}
resource "aws_network_acl" "data" { vpc_id = aws_vpc.v.id }
resource "aws_network_acl_association" "data" {
  network_acl_id = aws_network_acl.data.id
  subnet_id      = aws_subnet.data.id
}
resource "aws_network_acl_rule" "data_in" {
  network_acl_id = aws_network_acl.data.id
  rule_number    = 100
  egress         = false
  protocol       = "tcp"
  rule_action    = "allow"
  cidr_block     = "10.0.0.0/16"
  from_port      = 5432
  to_port        = 5432
}
resource "aws_network_acl_rule" "data_out" {
  network_acl_id = aws_network_acl.data.id
  rule_number    = 100
  egress         = true
  protocol       = "-1"
  rule_action    = "allow"
  cidr_block     = "10.0.0.0/16"
}
resource "aws_default_network_acl" "d" {
  default_network_acl_id = aws_vpc.v.default_network_acl_id
  ingress {
    rule_no    = 100
    action     = "allow"
    protocol   = "-1"
    cidr_block = "0.0.0.0/0"
    from_port  = 0
    to_port    = 0
  }
  egress {
    rule_no    = 100
    action     = "allow"
    protocol   = "-1"
    cidr_block = "0.0.0.0/0"
    from_port  = 0
    to_port    = 0
  }
}

# security groups: inline rules and standalone rules
resource "aws_security_group" "lb" {
  vpc_id = aws_vpc.v.id
  ingress {
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
  egress {
    from_port   = 8080
    to_port     = 8080
    protocol    = "tcp"
    cidr_blocks = ["10.0.0.0/16"]
  }
}
resource "aws_security_group" "app" {
  vpc_id = aws_vpc.v.id
  egress {
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
}
resource "aws_security_group_rule" "app_in" {
  type                     = "ingress"
  security_group_id        = aws_security_group.app.id
  source_security_group_id = aws_security_group.lb.id
  from_port                = 8080
  to_port                  = 8080
  protocol                 = "tcp"
}
resource "aws_security_group_rule" "app_to_db" {
  type                     = "egress"
  security_group_id        = aws_security_group.app.id
  source_security_group_id = aws_security_group.db.id
  from_port                = 5432
  to_port                  = 5432
  protocol                 = "tcp"
}
resource "aws_security_group" "db" {
  vpc_id = aws_vpc.v.id
  ingress {
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [aws_security_group.app.id]
  }
}

# compute, load balancer with listener / target group / attachment, WAF, database
resource "aws_instance" "web" {
  subnet_id              = aws_subnet.app.id
  vpc_security_group_ids = [aws_security_group.app.id]
}
resource "aws_instance" "worker" {
  subnet_id                   = aws_subnet.pub.id
  vpc_security_group_ids      = [aws_security_group.app.id]
  associate_public_ip_address = true
}
resource "aws_eip" "worker" {
  domain   = "vpc"
  instance = aws_instance.worker.id
}
resource "aws_lb" "front" {
  name            = "front"
  internal        = false
  subnets         = [aws_subnet.pub.id]
  security_groups = [aws_security_group.lb.id]
}
resource "aws_lb_target_group" "tg" {
  port        = 8080
  protocol    = "HTTP"
  vpc_id      = aws_vpc.v.id
  target_type = "instance"
}
resource "aws_lb_listener" "https" {
  load_balancer_arn = aws_lb.front.arn
  port              = 443
  protocol          = "HTTP"
  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.tg.arn
  }
}
resource "aws_lb_target_group_attachment" "web" {
  target_group_arn = aws_lb_target_group.tg.arn
  target_id        = aws_instance.web.id
}
resource "aws_wafv2_web_acl" "w" {
  name  = "w"
  scope = "REGIONAL"
}
resource "aws_wafv2_web_acl_association" "assoc" {
  resource_arn = aws_lb.front.arn
  web_acl_arn  = aws_wafv2_web_acl.w.arn
}
resource "aws_db_subnet_group" "g" {
  name       = "g"
  subnet_ids = [aws_subnet.data.id]
}
resource "aws_db_instance" "d" {
  identifier             = "d"
  engine                 = "postgres"
  instance_class         = "db.r6g.large"
  storage_encrypted      = true
  multi_az               = true
  db_subnet_group_name   = aws_db_subnet_group.g.name
  vpc_security_group_ids = [aws_security_group.db.id]
}

# identity: inline policy, a managed policy in the bundle, a bucket policy
resource "aws_iam_role" "app" {
  name               = "app"
  assume_role_policy = jsonencode({
    Version   = "2012-10-17"
    Statement = [{ Effect = "Allow", Principal = { Service = "ec2.amazonaws.com" }, Action = "sts:AssumeRole" }]
  })
}
resource "aws_iam_role_policy" "inline" {
  name   = "inline"
  role   = aws_iam_role.app.id
  policy = jsonencode({
    Version   = "2012-10-17"
    Statement = [{ Effect = "Allow", Action = ["sqs:SendMessage"], Resource = "arn:aws:sqs:eu-west-1:123456789012:q" }]
  })
}
resource "aws_iam_policy" "extra" {
  name   = "extra"
  policy = jsonencode({
    Version   = "2012-10-17"
    Statement = [{ Effect = "Allow", Action = ["s3:GetObject"], Resource = "arn:aws:s3:::data/*" }]
  })
}
resource "aws_iam_role_policy_attachment" "extra" {
  role       = aws_iam_role.app.name
  policy_arn = aws_iam_policy.extra.arn
}
resource "aws_s3_bucket" "data" { bucket = "data" }
resource "aws_s3_bucket_policy" "data" {
  bucket = aws_s3_bucket.data.id
  policy = jsonencode({
    Version   = "2012-10-17"
    Statement = [{ Effect = "Deny", Principal = "*", Action = "s3:*", Resource = "arn:aws:s3:::data/*" }]
  })
}
