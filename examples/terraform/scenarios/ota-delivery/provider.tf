# azemu scenario: server-less OTA delivery (Blob + Key Vault sign + CDN).
#
# Points the official azurerm provider at a local azemu instance.
# Assumes azemu is running via `docker compose up -d` from the repo root.

terraform {
  required_providers {
    azurerm = {
      source = "hashicorp/azurerm"
      # Lower bound 4.35: classic CDN (azurerm_cdn_*) was removed at azurerm
      # 4.35 in favour of Front Door, which this scenario uses. Upper bound
      # < 4.36: pin to the 4.35.x line azemu's Front Door emulation was
      # validated against, matching static-site and staying below the storage /
      # Key Vault data-plane tightening in later 4.x that azemu's path-style
      # Azurite and *.vault.localhost endpoints do not yet satisfy (TODO.md M6
      # and Known Gaps). Lift the upper bound alongside static-site.
      version = ">= 4.35, < 4.36"
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
