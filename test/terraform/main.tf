# Example: using azemu as a local Azure emulator with the official azurerm provider.
#
# Usage:
#   1. cd into this project directory
#   2. flox activate          (loads Go, Terraform, ARM_* env vars)
#   3. azemu-start            (builds, starts server, trusts cert)
#   4. tf-init && tf-apply    (terraform init && apply)
#   5. tf-destroy             (clean up)
#   6. azemu-stop             (stop server)
#
# The flox environment sets ARM_* variables to route azurerm to azemu.
# Do NOT set metadata_host - it triggers Azure Stack classification.

terraform {
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 4.0"
    }
  }
}

provider "azurerm" {
  features {}

  # azurerm reads ARM_* env vars from flox manifest to point at azemu.
  # We only need resource_provider_registrations here to skip registration.
  resource_provider_registrations = "none"

  # These credentials are overridden by flox env vars. They exist here
  # so terraform validate succeeds even outside the flox environment.
  subscription_id = "00000000-0000-0000-0000-000000000000"
  tenant_id       = "00000000-0000-0000-0000-000000000001"
  client_id       = "00000000-0000-0000-0000-000000000002"
  client_secret   = "azemu-mock-secret"
}

# Test: create a resource group locally
resource "azurerm_resource_group" "example" {
  name     = "azemu-test-rg"
  location = "uksouth"

  tags = {
    environment = "local"
    managed_by  = "azemu"
  }
}

output "resource_group_id" {
  value = azurerm_resource_group.example.id
}
