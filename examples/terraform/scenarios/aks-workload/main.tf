# AKS workload: networking + AKS cluster + managed identity + Key Vault.
# Represents the management-plane provisioning for a Kubernetes workload
# that reads secrets from Key Vault via workload identity.

resource "azurerm_resource_group" "main" {
  name     = "${var.prefix}-rg"
  location = var.location
}

# --- Networking ---

resource "azurerm_virtual_network" "main" {
  name                = "${var.prefix}-vnet"
  location            = var.location
  resource_group_name = azurerm_resource_group.main.name

  address_space = ["10.1.0.0/16"]
}

resource "azurerm_subnet" "aks" {
  name                 = "aks-nodes"
  resource_group_name  = azurerm_resource_group.main.name
  virtual_network_name = azurerm_virtual_network.main.name
  address_prefixes     = ["10.1.0.0/22"]
}

# --- AKS Cluster ---

resource "azurerm_kubernetes_cluster" "main" {
  name                = "${var.prefix}-cluster"
  location            = var.location
  resource_group_name = azurerm_resource_group.main.name
  dns_prefix          = var.prefix

  kubernetes_version = var.kubernetes_version

  default_node_pool {
    name           = "default"
    node_count     = 3
    vm_size        = "Standard_DS2_v2"
    vnet_subnet_id = azurerm_subnet.aks.id
  }

  identity {
    type = "SystemAssigned"
  }

  network_profile {
    network_plugin = "azure"
    service_cidr   = "10.2.0.0/16"
    dns_service_ip = "10.2.0.10"
  }
}

# --- Managed Identity (for workload identity federation) ---

resource "azurerm_user_assigned_identity" "workload" {
  name                = "${var.prefix}-workload-id"
  location            = var.location
  resource_group_name = azurerm_resource_group.main.name
}

# --- Key Vault (secrets for the workload) ---

resource "azurerm_key_vault" "main" {
  name                = "${var.prefix}-kv"
  location            = var.location
  resource_group_name = azurerm_resource_group.main.name
  tenant_id           = "00000000-0000-0000-0000-000000000001"
  sku_name            = "standard"

  purge_protection_enabled   = false
  soft_delete_retention_days = 7
}

resource "azurerm_key_vault_secret" "app_config" {
  name         = "app-connection-string"
  value        = "Server=db.internal;Database=myapp;Trusted_Connection=true;"
  key_vault_id = azurerm_key_vault.main.id
}
