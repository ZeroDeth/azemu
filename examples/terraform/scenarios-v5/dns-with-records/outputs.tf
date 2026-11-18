output "resource_group_id" {
  description = "Resource group ARM ID"
  value       = azurerm_resource_group.dns.id
}

output "dns_zone_id" {
  description = "DNS zone ARM ID"
  value       = azurerm_dns_zone.main.id
}

output "dns_zone_name_servers" {
  description = "Name servers assigned to the DNS zone by azemu"
  value       = azurerm_dns_zone.main.name_servers
}

output "a_record_id" {
  description = "A record ARM ID"
  value       = azurerm_dns_a_record.app.id
}

output "cname_record_fqdn" {
  description = "CNAME record FQDN (zone name appended by azemu)"
  value       = azurerm_dns_cname_record.api.fqdn
}
