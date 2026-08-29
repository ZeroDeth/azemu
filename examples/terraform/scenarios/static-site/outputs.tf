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
  description = "Front Door profile ARM ID"
  value       = azurerm_cdn_frontdoor_profile.site.id
}

output "cdn_endpoint_fqdn" {
  description = "Front Door endpoint hostname ({name}.azurefd.net)"
  value       = azurerm_cdn_frontdoor_endpoint.site.host_name
}

output "dns_zone_id" {
  description = "DNS zone ARM ID"
  value       = azurerm_dns_zone.site.id
}

output "www_cname_fqdn" {
  description = "CNAME record FQDN for www subdomain"
  value       = azurerm_dns_cname_record.frontdoor.fqdn
}
