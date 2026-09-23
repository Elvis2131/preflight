resource "aws_lb" "payments" {
  internal = false
}

resource "aws_db_instance" "payments" {
  engine   = "postgres"
  multi_az = true
}
