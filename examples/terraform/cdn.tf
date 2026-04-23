resource "azurerm_cdn_profile" "example" {
  name                = "examplecdnprofile"
  resource_group_name = azurerm_resource_group.example.name
  location            = "global"
  sku                 = "Standard_Microsoft"

  tags = {
    environment = "dev"
  }
}

resource "azurerm_cdn_endpoint" "example" {
  name                = "examplecdnendpoint"
  profile_name        = azurerm_cdn_profile.example.name
  resource_group_name = azurerm_resource_group.example.name
  location            = azurerm_resource_group.example.location

  origin {
    name      = "example-origin"
    host_name = "www.example.com"
  }

  tags = {
    environment = "dev"
  }
}
