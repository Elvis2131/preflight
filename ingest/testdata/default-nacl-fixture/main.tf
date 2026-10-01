# PC-149: a VPC whose default NACL is adopted and given rules (aws_default_network_acl),
# plus a subnet with NO aws_network_acl_association — so the subnet's NACL is the
# declared default, not an assumed one.

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

resource "aws_default_network_acl" "d" {
  default_network_acl_id = aws_vpc.v.default_network_acl_id

  ingress {
    rule_number = 100
    protocol    = "tcp"
    rule_action = "allow"
    cidr_block  = "0.0.0.0/0"
    from_port   = 443
    to_port     = 443
  }
}
