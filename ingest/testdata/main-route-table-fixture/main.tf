# PC-151: a VPC whose main route table is DECLARED (aws_default_route_table, with a default
# route to an internet gateway), a subnet with NO explicit association — so it uses that table
# and is public — and a second VPC with a custom table made main by
# aws_main_route_table_association.
resource "aws_vpc" "v" {
  cidr_block = "10.0.0.0/16"
}

resource "aws_internet_gateway" "igw" {
  vpc_id = aws_vpc.v.id
}

resource "aws_route53_record" "entry" {}

resource "aws_db_instance" "db" {}

resource "aws_subnet" "implicit" {
  vpc_id     = aws_vpc.v.id
  cidr_block = "10.0.1.0/24"
}

resource "aws_default_route_table" "main" {
  default_route_table_id = aws_vpc.v.default_route_table_id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.igw.id
  }
}

resource "aws_vpc" "w" {
  cidr_block = "10.1.0.0/16"
}

resource "aws_subnet" "other" {
  vpc_id     = aws_vpc.w.id
  cidr_block = "10.1.1.0/24"
}

resource "aws_route_table" "custom_main" {
  vpc_id = aws_vpc.w.id
}

resource "aws_main_route_table_association" "m" {
  vpc_id         = aws_vpc.w.id
  route_table_id = aws_route_table.custom_main.id
}
