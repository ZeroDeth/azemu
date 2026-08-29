output "resource_group_id" {
  description = "Resource group ARM ID"
  value       = azurerm_resource_group.main.id
}

output "storage_account_name" {
  description = "Storage account name (path-style Azurite account segment)"
  value       = azurerm_storage_account.ota.name
}

output "storage_account_id" {
  description = "Storage account ARM ID"
  value       = azurerm_storage_account.ota.id
}

output "storage_primary_blob_endpoint" {
  description = "Blob endpoint the publish script uploads to (Azurite path-style)"
  value       = azurerm_storage_account.ota.primary_blob_endpoint
}

output "key_vault_uri" {
  description = "Vault data-plane base URL ({vault}.vault.localhost form)"
  value       = azurerm_key_vault.signing.vault_uri
}

output "manifest_key_id" {
  description = "Versioned key ID (kid) the publish script signs with"
  value       = azurerm_key_vault_key.manifest.id
}

output "manifest_key_public_pem" {
  description = "Public key (PEM) the client embeds to verify the manifest signature"
  value       = azurerm_key_vault_key.manifest.public_key_pem
}

output "cdn_profile_id" {
  description = "Front Door profile ARM ID"
  value       = azurerm_cdn_frontdoor_profile.ota.id
}

output "cdn_endpoint_fqdn" {
  description = "Front Door endpoint hostname ({name}.azurefd.net); the read-path host the client fetches"
  value       = azurerm_cdn_frontdoor_endpoint.ota.host_name
}
