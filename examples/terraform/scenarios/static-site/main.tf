# Static site: Storage account + blob container behind a CDN profile
# with a DNS zone for custom domain records.

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

# --- CDN ---

resource "azurerm_cdn_profile" "site" {
  name                = "${var.prefix}-cdn"
  location            = var.location
  resource_group_name = azurerm_resource_group.main.name
  sku                 = "Standard_Microsoft"
}

resource "azurerm_cdn_endpoint" "site" {
  name                = "${var.prefix}-endpoint"
  profile_name        = azurerm_cdn_profile.site.name
  location            = var.location
  resource_group_name = azurerm_resource_group.main.name

  origin {
    name      = "storage-origin"
    host_name = "${azurerm_storage_account.site.name}.blob.core.windows.net"
  }

  origin_host_header = "${azurerm_storage_account.site.name}.blob.core.windows.net"
}

# --- DNS ---

resource "azurerm_dns_zone" "site" {
  name                = var.zone_name
  resource_group_name = azurerm_resource_group.main.name
}

resource "azurerm_dns_cname_record" "cdn" {
  name                = "www"
  zone_name           = azurerm_dns_zone.site.name
  resource_group_name = azurerm_resource_group.main.name
  ttl                 = 300
  record              = azurerm_cdn_endpoint.site.fqdn
}

resource "azurerm_dns_txt_record" "verification" {
  name                = "cdnverify"
  zone_name           = azurerm_dns_zone.site.name
  resource_group_name = azurerm_resource_group.main.name
  ttl                 = 300

  record {
    value = "cdnverify.${azurerm_cdn_endpoint.site.fqdn}"
  }
}
