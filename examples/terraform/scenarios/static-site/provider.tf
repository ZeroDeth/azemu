# azemu scenario: static site with CDN and DNS
#
# Points the official azurerm provider at a local azemu instance.
# Assumes azemu is running via `docker compose up -d` from the repo root.

terraform {
  required_providers {
    azurerm = {
      source = "hashicorp/azurerm"
      # Lower bound 4.35: the scenario uses cdn_frontdoor_*, and 4.35 is the
      # release azemu's Front Door emulation was developed against. The
      # resources exist earlier, so this is "tested against", not "requires".
      #
      # Upper bound < 4.36: the same, from the other side. Nothing in this
      # scenario is known to break above it; the pin keeps the scenario on the
      # exact provider the emulation was validated with. Note this scenario
      # uses storage_account_id, so the storage data-plane parser that forced
      # the < 4.35 pin elsewhere does not apply here.
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
