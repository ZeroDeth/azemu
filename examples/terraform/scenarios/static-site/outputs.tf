output "resource_group_id" {
  description = "Resource group ARM ID"
  value       = azurerm_resource_group.main.id
}

output "storage_account_id" {
  description = "Storage account ARM ID"
  value       = azurerm_storage_account.site.id
}

output "storage_primary_blob_endpoint" {
  description = "Primary blob endpoint URL"
  value       = azurerm_storage_account.site.primary_blob_endpoint
}

output "cdn_profile_id" {
  description = "CDN profile ARM ID"
  value       = azurerm_cdn_profile.site.id
}

output "cdn_endpoint_fqdn" {
  description = "CDN endpoint hostname"
  value       = azurerm_cdn_endpoint.site.fqdn
}

output "dns_zone_id" {
  description = "DNS zone ARM ID"
  value       = azurerm_dns_zone.site.id
}

output "www_cname_fqdn" {
  description = "CNAME record FQDN for www subdomain"
  value       = azurerm_dns_cname_record.cdn.fqdn
}
