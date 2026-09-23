resource "aws_db_instance" "payments" {
  engine         = "postgres"
  instance_class = "db.r6g.large"

  dynamic "timeouts" {
    for_each = ["create", "update", "delete"]
    content {
      create = "60m"
    }
  }
}
