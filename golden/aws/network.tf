# Network substrate. Not one of the eight golden node types itself, but every one of them
# is placed in it - a load_balancer or managed_database with no subnet/AZ identity cannot
# be reasoned about for AZ loss (§14 scenario 1).
#
# Three AZs, one NAT gateway per AZ. The clean baseline deliberately has NO single-AZ
# chokepoint: the broken variant in ../aws-broken collapses this to one AZ.

resource "aws_vpc" "payments" {
  cidr_block           = var.vpc_cidr
  enable_dns_support   = true
  enable_dns_hostnames = true

  tags = {
    Name       = "payments-vpc"
    Service    = "payments-api"
    Tier       = "tier1"
    Compliance = "PCI"
  }
}

resource "aws_internet_gateway" "payments" {
  vpc_id = aws_vpc.payments.id

  tags = {
    Name = "payments-igw"
  }
}

# --- Public subnets: ALB only ---------------------------------------------------------

resource "aws_subnet" "public_a" {
  vpc_id                  = aws_vpc.payments.id
  cidr_block              = "10.0.0.0/24"
  availability_zone       = local.az_a
  map_public_ip_on_launch = false

  tags = {
    Name = "payments-public-a"
    Tier = "public"
  }
}

resource "aws_subnet" "public_b" {
  vpc_id                  = aws_vpc.payments.id
  cidr_block              = "10.0.1.0/24"
  availability_zone       = local.az_b
  map_public_ip_on_launch = false

  tags = {
    Name = "payments-public-b"
    Tier = "public"
  }
}

resource "aws_subnet" "public_c" {
  vpc_id                  = aws_vpc.payments.id
  cidr_block              = "10.0.2.0/24"
  availability_zone       = local.az_c
  map_public_ip_on_launch = false

  tags = {
    Name = "payments-public-c"
    Tier = "public"
  }
}

# --- Private subnets: EKS workload ----------------------------------------------------

resource "aws_subnet" "private_a" {
  vpc_id            = aws_vpc.payments.id
  cidr_block        = "10.0.10.0/24"
  availability_zone = local.az_a

  tags = {
    Name = "payments-private-a"
    Tier = "private"
  }
}

resource "aws_subnet" "private_b" {
  vpc_id            = aws_vpc.payments.id
  cidr_block        = "10.0.11.0/24"
  availability_zone = local.az_b

  tags = {
    Name = "payments-private-b"
    Tier = "private"
  }
}

resource "aws_subnet" "private_c" {
  vpc_id            = aws_vpc.payments.id
  cidr_block        = "10.0.12.0/24"
  availability_zone = local.az_c

  tags = {
    Name = "payments-private-c"
    Tier = "private"
  }
}

# --- Data subnets: RDS + ElastiCache --------------------------------------------------

resource "aws_subnet" "data_a" {
  vpc_id            = aws_vpc.payments.id
  cidr_block        = "10.0.20.0/24"
  availability_zone = local.az_a

  tags = {
    Name = "payments-data-a"
    Tier = "data"
  }
}

resource "aws_subnet" "data_b" {
  vpc_id            = aws_vpc.payments.id
  cidr_block        = "10.0.21.0/24"
  availability_zone = local.az_b

  tags = {
    Name = "payments-data-b"
    Tier = "data"
  }
}

resource "aws_subnet" "data_c" {
  vpc_id            = aws_vpc.payments.id
  cidr_block        = "10.0.22.0/24"
  availability_zone = local.az_c

  tags = {
    Name = "payments-data-c"
    Tier = "data"
  }
}

# --- Egress: one NAT gateway per AZ, so a single AZ loss is not a total egress loss ----

resource "aws_eip" "nat_a" {
  domain = "vpc"

  tags = {
    Name = "payments-nat-eip-a"
  }
}

resource "aws_eip" "nat_b" {
  domain = "vpc"

  tags = {
    Name = "payments-nat-eip-b"
  }
}

resource "aws_eip" "nat_c" {
  domain = "vpc"

  tags = {
    Name = "payments-nat-eip-c"
  }
}

resource "aws_nat_gateway" "nat_a" {
  allocation_id = aws_eip.nat_a.id
  subnet_id     = aws_subnet.public_a.id
  depends_on    = [aws_internet_gateway.payments]

  tags = {
    Name = "payments-nat-a"
  }
}

resource "aws_nat_gateway" "nat_b" {
  allocation_id = aws_eip.nat_b.id
  subnet_id     = aws_subnet.public_b.id
  depends_on    = [aws_internet_gateway.payments]

  tags = {
    Name = "payments-nat-b"
  }
}

resource "aws_nat_gateway" "nat_c" {
  allocation_id = aws_eip.nat_c.id
  subnet_id     = aws_subnet.public_c.id
  depends_on    = [aws_internet_gateway.payments]

  tags = {
    Name = "payments-nat-c"
  }
}

# --- Route tables ---------------------------------------------------------------------

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.payments.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.payments.id
  }

  tags = {
    Name = "payments-rt-public"
  }
}

resource "aws_route_table_association" "public_a" {
  subnet_id      = aws_subnet.public_a.id
  route_table_id = aws_route_table.public.id
}

resource "aws_route_table_association" "public_b" {
  subnet_id      = aws_subnet.public_b.id
  route_table_id = aws_route_table.public.id
}

resource "aws_route_table_association" "public_c" {
  subnet_id      = aws_subnet.public_c.id
  route_table_id = aws_route_table.public.id
}

resource "aws_route_table" "private_a" {
  vpc_id = aws_vpc.payments.id

  route {
    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.nat_a.id
  }

  tags = {
    Name = "payments-rt-private-a"
  }
}

resource "aws_route_table" "private_b" {
  vpc_id = aws_vpc.payments.id

  route {
    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.nat_b.id
  }

  tags = {
    Name = "payments-rt-private-b"
  }
}

resource "aws_route_table" "private_c" {
  vpc_id = aws_vpc.payments.id

  route {
    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.nat_c.id
  }

  tags = {
    Name = "payments-rt-private-c"
  }
}

resource "aws_route_table_association" "private_a" {
  subnet_id      = aws_subnet.private_a.id
  route_table_id = aws_route_table.private_a.id
}

resource "aws_route_table_association" "private_b" {
  subnet_id      = aws_subnet.private_b.id
  route_table_id = aws_route_table.private_b.id
}

resource "aws_route_table_association" "private_c" {
  subnet_id      = aws_subnet.private_c.id
  route_table_id = aws_route_table.private_c.id
}
