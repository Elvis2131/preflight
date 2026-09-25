# PC-111's own synthetic fixture: a minimal, self-contained bundle exercising both
# real route shapes (aws_route_table's own inline route{} block, and a standalone
# aws_route resource) plus one unsupported route target (transit_gateway_id) — the
# real Terraform shape, not the golden bundle's own AWS account, so the unsupported-
# target case (never exercised by golden/aws) has its own real coverage.

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

resource "aws_internet_gateway" "igw" {
  vpc_id = aws_vpc.v.id
}

resource "aws_ec2_transit_gateway" "tgw" {}

# Inline route{} block shape (matches golden/aws's own real usage).
resource "aws_route_table" "rt" {
  vpc_id = aws_vpc.v.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.igw.id
  }
}

# Standalone aws_route resource shape (the Card names it explicitly; golden/aws
# doesn't use it, so this fixture is this shape's only real coverage).
resource "aws_route" "unsupported" {
  route_table_id     = aws_route_table.rt.id
  destination_cidr_block = "10.100.0.0/16"
  transit_gateway_id = aws_ec2_transit_gateway.tgw.id
}

resource "aws_route_table_association" "a" {
  subnet_id      = aws_subnet.s.id
  route_table_id = aws_route_table.rt.id
}
