# Server-less OTA delivery: signed update bundles in Blob Storage, manifest
# signed with a Key Vault key, fronted by a CDN. There is no compute on the
# read path. A build pipeline reads the signing key from Key Vault, builds and
# signs a manifest, and writes immutable artefacts to Blob; a release pipeline
# promotes a version by a server-side blob copy and writes rollout.json; the
# CDN serves the static files to clients.
#
# The blob container is created by the publish script, not Terraform: azemu
# does not mirror ARM containers into the Azurite data plane, so the publish
# script issues one Create Container call against Azurite before the first
# upload. The "${prefix}sa" account ships pre-registered in docker-compose.yml
# AZURITE_ACCOUNTS. See docs/SETUP.md "Storage account names and
# AZURITE_ACCOUNTS".

resource "azurerm_resource_group" "main" {
  name     = "${var.prefix}-rg"
  location = var.location
}

# --- Artefact storage ---

resource "azurerm_storage_account" "ota" {
  name                     = "${var.prefix}sa"
  resource_group_name      = azurerm_resource_group.main.name
  location                 = var.location
  account_tier             = "Standard"
  account_replication_type = "LRS"
  account_kind             = "StorageV2"

  # Anonymous read on the artefacts; clients download the manifest and assets
  # without credentials, through the CDN.
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

  # Sign-only pipeline: the publish script calls the Key Vault sign API with an
  # RS256 digest; the client verifies with the public key baked into the build.
  # The release (promote) identity has no Key Vault access at all, which is the
  # security boundary of this model.
  key_opts = ["sign", "verify"]
}

# --- CDN read path ---

resource "azurerm_cdn_profile" "ota" {
  name                = "${var.prefix}-cdn"
  location            = var.location
  resource_group_name = azurerm_resource_group.main.name
  sku                 = "Standard_Microsoft"
}

resource "azurerm_cdn_endpoint" "ota" {
  name                = "${var.prefix}-endpoint"
  profile_name        = azurerm_cdn_profile.ota.name
  location            = var.location
  resource_group_name = azurerm_resource_group.main.name

  # The Blob origin. azemu's CDN content data plane parses the storage account
  # from this host and reverse-proxies to Azurite path-style, passing the
  # origin's Content-Type and Cache-Control through unchanged.
  origin {
    name      = "blob-origin"
    host_name = "${azurerm_storage_account.ota.name}.blob.core.windows.net"
  }

  origin_host_header = "${azurerm_storage_account.ota.name}.blob.core.windows.net"
}
