resource "aws_db_instance" "replica" {
  count          = 3
  engine         = "postgres"
  instance_class = "db.r6g.large"
}
