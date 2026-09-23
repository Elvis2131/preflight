# GOLDEN NODE TYPE 7/8: queue/stream.
resource "azurerm_servicebus_namespace" "payments" {
  name                = "payments-servicebus"
  resource_group_name = azurerm_resource_group.payments.name
  location            = azurerm_resource_group.payments.location
  sku                 = "Standard"
}

resource "azurerm_servicebus_queue" "settlement" {
  name         = "settlement"
  namespace_id = azurerm_servicebus_namespace.payments.id

  dead_lettering_on_message_expiration = true
}
