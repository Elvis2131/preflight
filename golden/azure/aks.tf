# GOLDEN NODE TYPE 4/8: container_workload.
resource "azurerm_kubernetes_cluster" "payments" {
  name                = "payments-aks"
  resource_group_name = azurerm_resource_group.payments.name
  location            = azurerm_resource_group.payments.location
  dns_prefix          = "payments-aks"

  # Public API server — mirrors golden/aws/eks.tf's own broken-variant defect 5
  # shape (this is the CLEAN bundle; private_cluster_enabled = true here, matching
  # EKS's endpoint_public_access = false in the clean AWS bundle).
  private_cluster_enabled = true

  default_node_pool {
    name           = "default"
    node_count     = 3
    vm_size        = "Standard_D2_v2"
    vnet_subnet_id = azurerm_subnet.aks.id
  }

  identity {
    type = "SystemAssigned"
  }
}
