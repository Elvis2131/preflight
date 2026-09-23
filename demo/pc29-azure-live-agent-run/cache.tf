# GOLDEN NODE TYPE 6/8: cache.
resource "azurerm_redis_cache" "payments" {
  name                = "payments-redis"
  resource_group_name = azurerm_resource_group.payments.name
  location            = azurerm_resource_group.payments.location
  capacity            = 1
  family              = "P"
  sku_name            = "Premium"

  non_ssl_port_enabled = false

  redis_configuration {
  }
}
