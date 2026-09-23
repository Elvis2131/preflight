# GOLDEN NODE TYPE 8/8: identity.
#
# No trust_policy_present-equivalent capability exists here — see
# providers/azure/role_assignment.yaml's own doc comment for the real model
# difference this reflects, verified before writing that mapping, not discovered
# after.
resource "azurerm_role_assignment" "aks_to_rg" {
  scope                = azurerm_resource_group.payments.id
  principal_id         = azurerm_kubernetes_cluster.payments.identity[0].principal_id
  role_definition_name = "Contributor"
}
