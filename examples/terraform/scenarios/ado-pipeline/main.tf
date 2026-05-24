# ADO pipeline: ARM resources for workload identity federation.
#
# Provisions the Azure-side resources that an Azure DevOps pipeline
# needs to authenticate via workload identity (OIDC) without secrets.
# The ADO service connection itself is created via the ADO REST API
# on azemu's :4569 endpoint, not via Terraform.

resource "azurerm_resource_group" "main" {
  name     = "${var.prefix}-rg"
  location = var.location
}

# --- Managed Identity + Federated Credential ---

resource "azurerm_user_assigned_identity" "pipeline" {
  name                = "${var.prefix}-pipeline-id"
  location            = var.location
  resource_group_name = azurerm_resource_group.main.name
}

resource "azurerm_federated_identity_credential" "ado" {
  name                = "ado-federation"
  resource_group_name = azurerm_resource_group.main.name
  parent_id           = azurerm_user_assigned_identity.pipeline.id
  audience            = ["api://AzureADTokenExchange"]
  issuer              = var.ado_org_url
  subject             = "sc://azemu-org/azemu-project/azemu-service-connection"
}

# --- Key Vault (pipeline secrets) ---

resource "azurerm_key_vault" "pipeline" {
  name                = "${var.prefix}-kv"
  location            = var.location
  resource_group_name = azurerm_resource_group.main.name
  tenant_id           = "00000000-0000-0000-0000-000000000001"
  sku_name            = "standard"

  purge_protection_enabled   = false
  soft_delete_retention_days = 7
}

resource "azurerm_key_vault_secret" "deploy_token" {
  name         = "deploy-token"
  value        = "azemu-mock-deploy-token-value"
  key_vault_id = azurerm_key_vault.pipeline.id
}

# --- Storage (pipeline artifacts) ---

resource "azurerm_storage_account" "artifacts" {
  # Storage account names must be lowercase letters and numbers only (no hyphens).
  name                     = "${replace(var.prefix, "-", "")}artifacts"
  resource_group_name      = azurerm_resource_group.main.name
  location                 = var.location
  account_tier             = "Standard"
  account_replication_type = "LRS"
  account_kind             = "StorageV2"
}

resource "azurerm_storage_container" "artifacts" {
  name                  = "pipeline-artifacts"
  storage_account_id    = azurerm_storage_account.artifacts.id
  container_access_type = "private"
}
