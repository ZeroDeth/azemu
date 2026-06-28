resource "azurerm_resource_group" "redis" {
  name     = "${var.prefix}-rg"
  location = var.location
}

resource "azurerm_redis_cache" "cache" {
  name                 = "${var.prefix}-cache"
  resource_group_name  = azurerm_resource_group.redis.name
  location             = azurerm_resource_group.redis.location
  capacity             = 1
  family               = "C"
  sku_name             = "Standard"
  non_ssl_port_enabled = false
  minimum_tls_version  = "1.2"

  redis_configuration {
    maxmemory_policy = "allkeys-lru"
  }

  tags = {
    environment = "dev"
    scenario    = "redis-cache"
  }
}

resource "azurerm_key_vault" "main" {
  name                = "${var.prefix}-kv"
  location            = azurerm_resource_group.redis.location
  resource_group_name = azurerm_resource_group.redis.name
  tenant_id           = "00000000-0000-0000-0000-000000000001"
  sku_name            = "standard"

  purge_protection_enabled   = false
  soft_delete_retention_days = 7
}

# A real app reads the cache connection string from Key Vault rather than
# embedding it. azemu returns deterministic dev keys from the Redis listKeys
# endpoint, so the provider populates primary_connection_string.
resource "azurerm_key_vault_secret" "redis_connection" {
  name         = "redis-connection-string"
  value        = azurerm_redis_cache.cache.primary_connection_string
  key_vault_id = azurerm_key_vault.main.id
}
