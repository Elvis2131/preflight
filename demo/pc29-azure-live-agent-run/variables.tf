variable "dns_zone_name" {
  type    = string
  default = "payments.example.com"
}

variable "sql_admin_login" {
  type    = string
  default = "sqladmin"
}

variable "sql_admin_password" {
  type      = string
  default   = "ChangeMe-Not-A-Real-Secret-1!"
  sensitive = true
}
