# GOLDEN NODE TYPE 3/8 (load_balancer) + WAF policy (network_boundary). App Gateway
# chosen over Front Door — see providers/azure/application_gateway.yaml's own doc
# comment for the explicit decision and why.

resource "azurerm_web_application_firewall_policy" "payments" {
  name                = "payments-waf-policy"
  resource_group_name = azurerm_resource_group.payments.name
  location            = azurerm_resource_group.payments.location

  managed_rules {
    managed_rule_set {
      type    = "OWASP"
      version = "3.2"
    }
  }

  policy_settings {
    enabled = true
    mode    = "Prevention"
  }
}

resource "azurerm_application_gateway" "payments" {
  name                = "payments-appgw"
  resource_group_name = azurerm_resource_group.payments.name
  location            = azurerm_resource_group.payments.location
  firewall_policy_id  = azurerm_web_application_firewall_policy.payments.id

  sku {
    name     = "WAF_v2"
    tier     = "WAF_v2"
    capacity = 2
  }

  gateway_ip_configuration {
    name      = "payments-gateway-ip-config"
    subnet_id = azurerm_subnet.appgw.id
  }

  frontend_port {
    name = "http-port"
    port = 80
  }

  frontend_ip_configuration {
    name                 = "payments-frontend-ip"
    public_ip_address_id = azurerm_public_ip.appgw.id
  }

  backend_address_pool {
    name = "payments-backend-pool"
  }

  backend_http_settings {
    name                  = "payments-backend-settings"
    cookie_based_affinity = "Disabled"
    port                  = 80
    protocol              = "Http"
    request_timeout       = 30
  }

  http_listener {
    name                           = "payments-http-listener"
    frontend_ip_configuration_name = "payments-frontend-ip"
    frontend_port_name             = "http-port"
    protocol                       = "Http"
  }

  request_routing_rule {
    name                       = "payments-routing-rule"
    rule_type                  = "Basic"
    http_listener_name         = "payments-http-listener"
    backend_address_pool_name  = "payments-backend-pool"
    backend_http_settings_name = "payments-backend-settings"
    priority                   = 100
  }
}
