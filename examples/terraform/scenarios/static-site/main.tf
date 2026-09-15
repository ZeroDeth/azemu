# Static site: Storage account + blob container behind an Azure Front Door
# profile with a DNS zone for custom domain records.

resource "azurerm_resource_group" "main" {
  name     = "${var.prefix}-rg"
  location = var.location
}

# --- Storage ---

resource "azurerm_storage_account" "site" {
  name                     = "${var.prefix}sa"
  resource_group_name      = azurerm_resource_group.main.name
  location                 = var.location
  account_tier             = "Standard"
  account_replication_type = "LRS"
  account_kind             = "StorageV2"
}

resource "azurerm_storage_container" "web" {
  name                  = "$web"
  storage_account_id    = azurerm_storage_account.site.id
  container_access_type = "blob"
}

# --- Front Door ---

resource "azurerm_cdn_frontdoor_profile" "site" {
  name                = "${var.prefix}-fd"
  resource_group_name = azurerm_resource_group.main.name
  sku_name            = "Standard_AzureFrontDoor"
}

resource "azurerm_cdn_frontdoor_endpoint" "site" {
  name                     = "${var.prefix}-endpoint"
  cdn_frontdoor_profile_id = azurerm_cdn_frontdoor_profile.site.id
}

resource "azurerm_cdn_frontdoor_origin_group" "site" {
  name                     = "${var.prefix}-og"
  cdn_frontdoor_profile_id = azurerm_cdn_frontdoor_profile.site.id

  load_balancing {
    sample_size                 = 4
    successful_samples_required = 3
  }
}

resource "azurerm_cdn_frontdoor_origin" "site" {
  name                          = "storage-origin"
  cdn_frontdoor_origin_group_id = azurerm_cdn_frontdoor_origin_group.site.id
  enabled                       = true

  host_name          = "${azurerm_storage_account.site.name}.blob.core.windows.net"
  origin_host_header = "${azurerm_storage_account.site.name}.blob.core.windows.net"
  http_port          = 80
  https_port         = 443
  priority           = 1
  weight             = 1000

  certificate_name_check_enabled = false
}

resource "azurerm_cdn_frontdoor_route" "site" {
  name                          = "${var.prefix}-route"
  cdn_frontdoor_endpoint_id     = azurerm_cdn_frontdoor_endpoint.site.id
  cdn_frontdoor_origin_group_id = azurerm_cdn_frontdoor_origin_group.site.id
  cdn_frontdoor_origin_ids      = [azurerm_cdn_frontdoor_origin.site.id]

  supported_protocols    = ["Http", "Https"]
  patterns_to_match      = ["/*"]
  forwarding_protocol    = "MatchRequest"
  link_to_default_domain = true
}

# --- DNS ---

resource "azurerm_dns_zone" "site" {
  name                = var.zone_name
  resource_group_name = azurerm_resource_group.main.name
}

resource "azurerm_dns_cname_record" "frontdoor" {
  name                = "www"
  zone_name           = azurerm_dns_zone.site.name
  resource_group_name = azurerm_resource_group.main.name
  ttl                 = 300
  record              = azurerm_cdn_frontdoor_endpoint.site.host_name
}
