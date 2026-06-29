# azemu scenario: static site with CDN and DNS
#
# Points the official azurerm provider at a local azemu instance.
# Assumes azemu is running via `docker compose up -d` from the repo root.

terraform {
  required_providers {
    azurerm = {
      source = "hashicorp/azurerm"
      # Lower bound 4.35: Front Door (cdn_frontdoor_*) replaced classic CDN,
      # which the provider removed at v4.35.0, so this scenario requires
      # >= 4.35. Upper bound < 4.36: this scenario also creates an
      # azurerm_storage_container against azemu's path-style Azurite endpoint.
      # The provider's storage data-plane parser rejects a non-core.windows.net
      # blob host, and the container resource tightened on this from v4.77
      # (storage_account_name deprecation) through the 4.78+ break recorded in
      # TODO.md M6. Pinning 4.35.x keeps the scenario on the exact provider
      # azemu's Front Door emulation was validated against, below that
      # tightening. Lift the upper bound once azemu serves
      # *.blob.core.windows.net blob endpoints (TODO.md Known Gaps).
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
