# GOLDEN NODE TYPE 5/8: managed_database.
#
# DEFECT (SPOF + RPO + PCI, tier1): mirrors golden/aws-broken/rds.tf's defect 2 shape
# exactly — zone_redundant=false (no synchronous secondary, the workload's RPO 0 claim
# is false) and transparent_data_encryption_enabled=false (PCI violation on a
# cardholder-data store). Expected: both the storage-encryption and RPO-feasibility
# findings differ from golden/azure's clean values.
resource "azurerm_mssql_server" "payments" {
  name                         = "payments-sql-server"
  resource_group_name          = azurerm_resource_group.payments.name
  location                     = azurerm_resource_group.payments.location
  version                      = "12.0"
  administrator_login          = var.sql_admin_login
  administrator_login_password = var.sql_admin_password
}

resource "azurerm_mssql_database" "payments" {
  name      = "payments-db"
  server_id = azurerm_mssql_server.payments.id
  sku_name  = "BC_Gen5_2" # Business Critical — required tier for zone_redundant

  zone_redundant                      = false
  transparent_data_encryption_enabled = false
}
