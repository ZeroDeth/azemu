output "resource_group_id" {
  description = "Resource group ARM ID"
  value       = azurerm_resource_group.example.id
}

output "vnet_id" {
  description = "Virtual network ARM ID"
  value       = azurerm_virtual_network.example.id
}

output "subnet_id" {
  description = "Subnet ARM ID"
  value       = azurerm_subnet.example.id
}

output "redis_cache_id" {
  description = "Redis cache ARM ID"
  value       = azurerm_redis_cache.example.id
}

output "redis_cache_hostname" {
  description = "Redis cache hostname (derived from AZEMU_REDIS_ENDPOINT)"
  value       = azurerm_redis_cache.example.hostname
}
