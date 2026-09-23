# GOLDEN NODE TYPE 5/8: managed_database.
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

  zone_redundant                      = true
  transparent_data_encryption_enabled = true
}
