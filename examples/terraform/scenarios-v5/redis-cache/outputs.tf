output "resource_group_id" {
  description = "Resource group ARM ID"
  value       = azurerm_resource_group.redis.id
}

output "redis_cache_id" {
  description = "Redis cache ARM ID"
  value       = azurerm_redis_cache.cache.id
}

output "redis_hostname" {
  description = "Redis cache hostname returned by azemu"
  value       = azurerm_redis_cache.cache.hostname
}

output "redis_ssl_port" {
  description = "Redis cache SSL port"
  value       = azurerm_redis_cache.cache.ssl_port
}

output "redis_primary_connection_string" {
  description = "Primary connection string (deterministic dev key from azemu)"
  value       = azurerm_redis_cache.cache.primary_connection_string
  sensitive   = true
}

output "key_vault_secret_id" {
  description = "ARM ID of the Key Vault secret holding the connection string"
  value       = azurerm_key_vault_secret.redis_connection.id
}
