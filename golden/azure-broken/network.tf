# Network substrate for the Azure golden bundle (PC-22). No per-AZ subnet layout here
# — deliberately, not a simplification of convenience: Azure subnets carry no zone
# attribute at all (verified against raw.githubusercontent.com/hashicorp/
# terraform-provider-azurerm's azurerm_subnet docs, 2026-09-21) — zone-redundancy in
# Azure is expressed per-RESOURCE (Application Gateway's own zone spanning, Azure
# SQL's zone_redundant, etc.), not per-subnet the way AWS's availability_zone is. AWS's
# golden bundle needs 9 subnets (3 tiers x 3 AZs) specifically to give PC-14's
# containment-based zone-kill engine real per-AZ nodes to act on; that engine has no
# Azure equivalent in this pass (see golden/azure/README.md), so replicating that
# subnet count here would be cargo-culting AWS's shape without AWS's reason for it.
# Two subnets, by TIER only (a real, independent-of-AZ segmentation reason: the edge
# vs the workload).

resource "azurerm_resource_group" "payments" {
  name     = "payments-api-rg"
  location = "westeurope"
}

resource "azurerm_virtual_network" "payments" {
  name                = "payments-vnet"
  resource_group_name = azurerm_resource_group.payments.name
  location            = azurerm_resource_group.payments.location
  address_space       = ["10.1.0.0/16"]
}

resource "azurerm_subnet" "appgw" {
  name                 = "payments-appgw-subnet"
  resource_group_name  = azurerm_resource_group.payments.name
  virtual_network_name = azurerm_virtual_network.payments.name
  address_prefixes     = ["10.1.0.0/24"]
}

resource "azurerm_subnet" "aks" {
  name                 = "payments-aks-subnet"
  resource_group_name  = azurerm_resource_group.payments.name
  virtual_network_name = azurerm_virtual_network.payments.name
  address_prefixes     = ["10.1.1.0/24"]
}

resource "azurerm_network_security_group" "appgw" {
  name                = "payments-appgw-nsg"
  resource_group_name = azurerm_resource_group.payments.name
  location            = azurerm_resource_group.payments.location

  # Required for Application Gateway v2 to receive Azure-managed control-plane traffic.
  # Terraform azurerm provider docs / Azure Application Gateway docs: v2 SKUs require
  # inbound allow on 65200-65535 from the GatewayManager service tag.
  security_rule {
    name                       = "AllowGatewayManager"
    priority                   = 100
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = "65200-65535"
    source_address_prefix      = "GatewayManager"
    destination_address_prefix = "*"
  }

  security_rule {
    name                       = "AllowHTTP"
    priority                   = 110
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = "80"
    source_address_prefix      = "Internet"
    destination_address_prefix = "*"
  }
}

resource "azurerm_subnet_network_security_group_association" "appgw" {
  subnet_id                 = azurerm_subnet.appgw.id
  network_security_group_id = azurerm_network_security_group.appgw.id
}

resource "azurerm_public_ip" "appgw" {
  name                = "payments-appgw-pip"
  resource_group_name = azurerm_resource_group.payments.name
  location            = azurerm_resource_group.payments.location
  allocation_method   = "Static"
  sku                 = "Standard"
}
