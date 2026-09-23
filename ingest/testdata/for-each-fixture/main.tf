resource "aws_db_instance" "replica" {
  for_each       = toset(["a", "b", "c"])
  engine         = "postgres"
  instance_class = "db.r6g.large"
}
