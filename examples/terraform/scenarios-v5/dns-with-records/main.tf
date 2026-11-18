resource "azurerm_resource_group" "dns" {
  name     = "${var.prefix}-rg"
  location = var.location
}

resource "azurerm_dns_zone" "main" {
  name                = var.zone_name
  resource_group_name = azurerm_resource_group.dns.name
}

resource "azurerm_dns_a_record" "app" {
  name                = "app"
  zone_name           = azurerm_dns_zone.main.name
  resource_group_name = azurerm_resource_group.dns.name
  ttl                 = 300
  records             = ["10.0.0.10", "10.0.0.11"]
}

resource "azurerm_dns_aaaa_record" "app_v6" {
  name                = "app"
  zone_name           = azurerm_dns_zone.main.name
  resource_group_name = azurerm_resource_group.dns.name
  ttl                 = 300
  records             = ["fd00::10", "fd00::11"]
}

resource "azurerm_dns_cname_record" "api" {
  name                = "api"
  zone_name           = azurerm_dns_zone.main.name
  resource_group_name = azurerm_resource_group.dns.name
  ttl                 = 300
  record              = "app.${var.zone_name}"
}

resource "azurerm_dns_txt_record" "spf" {
  name                = "@"
  zone_name           = azurerm_dns_zone.main.name
  resource_group_name = azurerm_resource_group.dns.name
  ttl                 = 300

  record {
    value = "v=spf1 include:_spf.example.com ~all"
  }
}

resource "azurerm_dns_mx_record" "mail" {
  name                = "@"
  zone_name           = azurerm_dns_zone.main.name
  resource_group_name = azurerm_resource_group.dns.name
  ttl                 = 300

  record {
    preference = 10
    exchange   = "mail.${var.zone_name}"
  }

  record {
    preference = 20
    exchange   = "mail2.${var.zone_name}"
  }
}
