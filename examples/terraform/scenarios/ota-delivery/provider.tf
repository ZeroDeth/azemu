# azemu scenario: server-less OTA delivery (Blob + Key Vault sign + CDN).
#
# Points the official azurerm provider at a local azemu instance.
# Assumes azemu is running via `docker compose up -d` from the repo root.

terraform {
  required_providers {
    azurerm = {
      source = "hashicorp/azurerm"
      # Classic CDN (azurerm_cdn_*) was removed at azurerm 4.35 in favour of
      # Front Door. This scenario uses classic CDN, so it pins below 4.35,
      # matching the static-site scenario and the rest of the suite.
      version = ">= 4.0, < 4.35"
    }
  }
}

provider "azurerm" {
  features {}

  # Point the provider at azemu's metadata endpoint.
  metadata_host = "127.0.0.1:4567"

  # Skip provider registration; azemu does not implement the full
  # Microsoft.Resources/providers surface.
  resource_provider_registrations = "none"

  # Mock credentials. azemu accepts any value.
  subscription_id = "00000000-0000-0000-0000-000000000000"
  tenant_id       = "00000000-0000-0000-0000-000000000001"
  client_id       = "00000000-0000-0000-0000-000000000002"
  client_secret   = "azemu-mock-secret"
}
