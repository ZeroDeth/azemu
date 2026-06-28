# OTA update pipeline: signed update bundles in Blob Storage, manifest
# signing key in Key Vault. No server in the runtime path; the app fetches
# manifest.json and bundle assets as static files over HTTP.
#
# The blob container is intentionally NOT managed here. azemu does not
# mirror ARM containers into the Azurite data plane, so the publish script
# creates the container directly against Azurite with
# `x-ms-blob-public-access: blob` before the first upload. See
# docs/SETUP.md "Storage account names and AZURITE_ACCOUNTS".

resource "azurerm_resource_group" "main" {
  name     = "${var.prefix}-rg"
  location = var.location
}

# --- Update bundle storage ---

resource "azurerm_storage_account" "updates" {
  name                     = "${var.prefix}sa"
  resource_group_name      = azurerm_resource_group.main.name
  location                 = var.location
  account_tier             = "Standard"
  account_replication_type = "LRS"
  account_kind             = "StorageV2"

  # Anonymous read on the updates container; the app downloads
  # manifest.json and assets without credentials.
  allow_nested_items_to_be_public = true
}

# --- Manifest signing ---

resource "azurerm_key_vault" "signing" {
  name                = "${var.prefix}kv"
  resource_group_name = azurerm_resource_group.main.name
  location            = var.location
  tenant_id           = "00000000-0000-0000-0000-000000000001"
  sku_name            = "standard"
}

resource "azurerm_key_vault_key" "manifest" {
  name         = "manifest-signing-key"
  key_vault_id = azurerm_key_vault.signing.id
  key_type     = "RSA"
  key_size     = 2048

  # Sign-only pipeline: the publish script calls the Key Vault sign API
  # with an RS256 digest; the app verifies with the public key baked into
  # the build (expo-updates code signing).
  key_opts = ["sign", "verify"]
}
