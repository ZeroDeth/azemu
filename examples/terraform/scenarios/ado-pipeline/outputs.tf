output "resource_group_id" {
  description = "Resource group ARM ID"
  value       = azurerm_resource_group.main.id
}

output "identity_id" {
  description = "Pipeline managed identity ARM ID"
  value       = azurerm_user_assigned_identity.pipeline.id
}

output "identity_client_id" {
  description = "Pipeline managed identity client ID"
  value       = azurerm_user_assigned_identity.pipeline.client_id
}

output "federated_credential_id" {
  description = "Federated identity credential ARM ID"
  value       = azurerm_federated_identity_credential.ado.id
}

output "key_vault_id" {
  description = "Key Vault ARM ID"
  value       = azurerm_key_vault.pipeline.id
}

output "key_vault_uri" {
  description = "Key Vault URI"
  value       = azurerm_key_vault.pipeline.vault_uri
}

output "storage_account_id" {
  description = "Storage account ARM ID for pipeline artifacts"
  value       = azurerm_storage_account.artifacts.id
}

output "secret_id" {
  description = "Key Vault secret ARM ID"
  value       = azurerm_key_vault_secret.deploy_token.id
}

output "storage_primary_blob_endpoint" {
  description = "Primary blob endpoint URL"
  value       = azurerm_storage_account.artifacts.primary_blob_endpoint
}

output "blob_container_name" {
  description = "Blob container name for pipeline artifacts"
  value       = azurerm_storage_container.artifacts.name
}
