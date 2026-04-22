resource "azurerm_key_vault" "example" {
  name                = "examplekeyvault001"
  resource_group_name = azurerm_resource_group.example.name
  location            = azurerm_resource_group.example.location
  tenant_id           = "00000000-0000-0000-0000-000000000001"
  sku_name            = "standard"

  soft_delete_retention_days = 90

  tags = {
    environment = "dev"
  }
}
