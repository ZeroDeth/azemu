output "resource_group_id" {
  description = "Resource group ARM ID"
  value       = azurerm_resource_group.main.id
}

output "aks_cluster_id" {
  description = "AKS cluster ARM ID"
  value       = azurerm_kubernetes_cluster.main.id
}

output "aks_fqdn" {
  description = "AKS cluster FQDN"
  value       = azurerm_kubernetes_cluster.main.fqdn
}

output "identity_id" {
  description = "User-assigned managed identity ARM ID"
  value       = azurerm_user_assigned_identity.workload.id
}

output "identity_client_id" {
  description = "Managed identity client ID"
  value       = azurerm_user_assigned_identity.workload.client_id
}

output "key_vault_id" {
  description = "Key Vault ARM ID"
  value       = azurerm_key_vault.main.id
}

output "key_vault_uri" {
  description = "Key Vault URI"
  value       = azurerm_key_vault.main.vault_uri
}

output "secret_id" {
  description = "Key Vault secret ARM ID"
  value       = azurerm_key_vault_secret.app_config.id
}
