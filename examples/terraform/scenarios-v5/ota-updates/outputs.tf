output "resource_group_id" {
  description = "Resource group ARM ID"
  value       = azurerm_resource_group.main.id
}

output "storage_account_id" {
  description = "Update-bundle storage account ARM ID"
  value       = azurerm_storage_account.updates.id
}

output "storage_primary_blob_endpoint" {
  description = "Blob endpoint the publish script uploads to (Azurite path-style)"
  value       = azurerm_storage_account.updates.primary_blob_endpoint
}

output "key_vault_id" {
  description = "Signing vault ARM ID"
  value       = azurerm_key_vault.signing.id
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
  description = "Public key (PEM) the app build embeds for verification"
  value       = azurerm_key_vault_key.manifest.public_key_pem
}
