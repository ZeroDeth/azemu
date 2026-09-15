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

# --- Front Door read path ---

resource "azurerm_cdn_frontdoor_profile" "ota" {
  name                = "${var.prefix}-fd"
  resource_group_name = azurerm_resource_group.main.name
  sku_name            = "Standard_AzureFrontDoor"
}

resource "azurerm_cdn_frontdoor_endpoint" "ota" {
  name                     = "${var.prefix}-endpoint"
  cdn_frontdoor_profile_id = azurerm_cdn_frontdoor_profile.ota.id
}

resource "azurerm_cdn_frontdoor_origin_group" "ota" {
  name                     = "${var.prefix}-og"
  cdn_frontdoor_profile_id = azurerm_cdn_frontdoor_profile.ota.id

  load_balancing {
    sample_size                 = 4
    successful_samples_required = 3
  }
}

# The Blob origin. azemu's Front Door content data plane walks endpoint -> route
# -> origin group -> origin, parses the storage account from this host, and
# reverse-proxies to Azurite path-style, passing the origin's Content-Type and
# Cache-Control through unchanged (the OTA manifest keeps its multipart boundary
# and short TTL).
resource "azurerm_cdn_frontdoor_origin" "ota" {
  name                          = "blob-origin"
  cdn_frontdoor_origin_group_id = azurerm_cdn_frontdoor_origin_group.ota.id
  enabled                       = true

  host_name          = "${azurerm_storage_account.ota.name}.blob.core.windows.net"
  origin_host_header = "${azurerm_storage_account.ota.name}.blob.core.windows.net"
  http_port          = 80
  https_port         = 443
  priority           = 1
  weight             = 1000

  certificate_name_check_enabled = false
}

resource "azurerm_cdn_frontdoor_route" "ota" {
  name                          = "${var.prefix}-route"
  cdn_frontdoor_endpoint_id     = azurerm_cdn_frontdoor_endpoint.ota.id
  cdn_frontdoor_origin_group_id = azurerm_cdn_frontdoor_origin_group.ota.id
  cdn_frontdoor_origin_ids      = [azurerm_cdn_frontdoor_origin.ota.id]

  supported_protocols    = ["Http", "Https"]
  patterns_to_match      = ["/*"]
  forwarding_protocol    = "MatchRequest"
  link_to_default_domain = true
}
