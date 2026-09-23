# GOLDEN NODE TYPE 1/8: dns.
#
# dns and load_balancer are structurally DISCONNECTED nodes in this bundle's IR —
# stated explicitly, not a bug to chase. Verified against
# raw.githubusercontent.com/hashicorp/terraform-provider-azurerm (2026-09-21):
# azurerm_dns_a_record's target_resource_id documented example aliases a Public IP
# resource, not an Application Gateway directly. azurerm_public_ip is not one of the 8
# golden mapped types (matching AWS's own aws_eip/aws_internet_gateway, also never
# separately mapped) — ingest only forms an edge when BOTH ends have a real mapped
# node, so the reference chain dns -> [public IP, unmapped] -> load_balancer does not
# produce a direct edge. This is a real Azure/AWS topological difference (Route 53's
# alias block can reference an ALB directly, one hop), not a mapping gap to force
# closed.
resource "azurerm_dns_zone" "payments" {
  name                = var.dns_zone_name
  resource_group_name = azurerm_resource_group.payments.name
}

resource "azurerm_dns_a_record" "api" {
  name                = "api"
  zone_name           = azurerm_dns_zone.payments.name
  resource_group_name = azurerm_resource_group.payments.name
  ttl                 = 300
  target_resource_id  = azurerm_public_ip.appgw.id
}
