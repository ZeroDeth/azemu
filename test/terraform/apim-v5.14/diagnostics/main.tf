# Independent CAF102 management-plane regression fixture. The parent directory
# remains the standalone three-resource CAF101 fixture.
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

resource "azurerm_resource_group" "diagnostics" {
  name     = "azemu-apim-diagnostics-514"
  location = "australiaeast"
}

resource "azurerm_api_management" "diagnostics" {
  name                = "azemu-apim-diagnostics-514"
  resource_group_name = azurerm_resource_group.diagnostics.name
  location            = azurerm_resource_group.diagnostics.location
  publisher_name      = "My Company"
  publisher_email     = "company@example.test"
  sku_name            = "Developer_1"
}

resource "azurerm_api_management_api" "example" {
  name                = "example-api"
  resource_group_name = azurerm_resource_group.diagnostics.name
  api_management_name = azurerm_api_management.diagnostics.name
  revision            = "1"
  display_name        = "Example API"
  path                = "example"
  protocols           = ["https"]

  # Like actual CAF101/102, the first plan may clear imported description and
  # terms omitted by the provider's final CREATE PUT. Do not mask that drift.
  import {
    content_format = "swagger-link-json"
    content_value  = "https://petstore.swagger.io/v2/swagger.json"
  }
}

resource "azurerm_application_insights" "diagnostics" {
  name                = "azemu-apim-diagnostics-514"
  resource_group_name = azurerm_resource_group.diagnostics.name
  location            = azurerm_resource_group.diagnostics.location
  application_type    = "web"
}

resource "azurerm_api_management_logger" "diagnostics" {
  name                = "example-apimlogger"
  resource_group_name = azurerm_resource_group.diagnostics.name
  api_management_name = azurerm_api_management.diagnostics.name
  application_insights {
    instrumentation_key = azurerm_application_insights.diagnostics.instrumentation_key
  }
}

resource "azurerm_api_management_api_diagnostic" "diagnostics" {
  identifier                = "applicationinsights"
  resource_group_name       = azurerm_resource_group.diagnostics.name
  api_management_name       = azurerm_api_management.diagnostics.name
  api_name                  = azurerm_api_management_api.example.name
  api_management_logger_id  = azurerm_api_management_logger.diagnostics.id
  sampling_percentage       = 5
  always_log_errors         = true
  log_client_ip             = true
  verbosity                 = "verbose"
  http_correlation_protocol = "W3C"

  frontend_request {
    body_bytes     = 32
    headers_to_log = ["content-type", "accept", "origin"]
  }
  frontend_response {
    body_bytes     = 32
    headers_to_log = ["content-type", "content-length", "origin"]
  }
  backend_request {
    body_bytes     = 32
    headers_to_log = ["content-type", "accept", "origin"]
  }
  backend_response {
    body_bytes     = 32
    headers_to_log = ["content-type", "content-length", "origin"]
  }
}
