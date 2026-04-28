resource "azurerm_redis_cache" "example" {
  name                 = "example-redis-001"
  resource_group_name  = azurerm_resource_group.example.name
  location             = azurerm_resource_group.example.location
  capacity             = 1
  family               = "C"
  sku_name             = "Standard"
  non_ssl_port_enabled = true
  minimum_tls_version  = "1.2"

  redis_configuration {
    maxmemory_policy = "allkeys-lru"
  }

  tags = {
    environment = "dev"
  }
}
