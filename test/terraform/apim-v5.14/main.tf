terraform {
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "= 5.14.0"
    }
  }
}

provider "azurerm" {
  features {}
  resource_provider_registrations = "none"
}

resource "azurerm_resource_group" "apim" {
  name     = "azemu-apim-basic-514"
  location = "australiaeast"
}

resource "azurerm_api_management" "basic" {
  name                = "azemu-apim-basic-514"
  resource_group_name = azurerm_resource_group.apim.name
  location            = azurerm_resource_group.apim.location
  publisher_name      = "My Company"
  publisher_email     = "company@example.test"
  sku_name            = "Developer_1"

  protocols {
    http2_enabled = true
  }

  tags = {
    scenario = "CAF apim/101-api"
  }
}

# Mirrors CAF101 without hiding imported metadata via ignore_changes.
# The immediate second plan currently exits 2; see TODO.md (CAF APIM101).
resource "azurerm_api_management_api" "example" {
  name                = "example-api"
  resource_group_name = azurerm_resource_group.apim.name
  api_management_name = azurerm_api_management.basic.name
  revision            = "1"
  display_name        = "Example API"
  path                = "example"
  protocols           = ["https"]

  import {
    content_format = "swagger-link-json"
    content_value  = "https://petstore.swagger.io/v2/swagger.json"
  }
}

resource "azurerm_api_management_backend" "example" {
  name                = "example-backend"
  resource_group_name = azurerm_resource_group.apim.name
  api_management_name = azurerm_api_management.basic.name
  protocol            = "http"
  url                 = "https://backend"

  tls {
    validate_certificate_chain = true
    validate_certificate_name  = false
  }
}
