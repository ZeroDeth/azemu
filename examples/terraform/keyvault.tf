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

resource "azurerm_key_vault_secret" "example" {
  name         = "example-secret"
  value        = "my-super-secret-value"
  key_vault_id = azurerm_key_vault.example.id

  tags = {
    environment = "dev"
  }

  depends_on = [azurerm_key_vault.example]
}

resource "azurerm_key_vault_key" "example" {
  name         = "example-signing-key"
  key_vault_id = azurerm_key_vault.example.id
  key_type     = "RSA"
  key_size     = 2048
  key_opts     = ["sign", "verify"]

  tags = {
    environment = "dev"
  }

  depends_on = [azurerm_key_vault.example]
}
