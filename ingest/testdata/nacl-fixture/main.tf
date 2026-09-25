# PC-113's own synthetic fixture: golden/aws has zero NACL resources (confirmed —
# PC-78/79 explicitly scoped NACLs out at the time), so this is this feature's only
# real ingest coverage. Exercises both real Terraform shapes: aws_network_acl's own
# inline ingress{}/egress{} blocks, and the standalone aws_network_acl_rule resource.

resource "aws_vpc" "v" {
  cidr_block = "10.0.0.0/16"
}

resource "aws_route53_record" "entry" {}

resource "aws_db_instance" "db" {}

resource "aws_subnet" "s" {
  vpc_id            = aws_vpc.v.id
  availability_zone = "eu-west-1a"
  cidr_block        = "10.0.0.0/24"
}

resource "aws_network_acl" "n" {
  vpc_id = aws_vpc.v.id

  ingress {
    rule_number = 100
    protocol    = "tcp"
    rule_action = "allow"
    cidr_block  = "0.0.0.0/0"
    from_port   = 443
    to_port     = 443
  }
}

resource "aws_network_acl_rule" "deny_ssh" {
  network_acl_id = aws_network_acl.n.id
  rule_number     = 90
  egress          = false
  protocol        = "tcp"
  rule_action     = "deny"
  cidr_block      = "198.51.100.0/24"
  from_port       = 22
  to_port         = 22
}

resource "aws_network_acl_association" "a" {
  subnet_id      = aws_subnet.s.id
  network_acl_id = aws_network_acl.n.id
}
