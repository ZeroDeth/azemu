# Example: using azemu as a local Azure emulator with the official azurerm provider.
#
# Prerequisites:
#   1. azemu running: ./bin/azemu (or docker run -p 4566:4566 -p 4567:4567 azemu)
#   2. Trust the self-signed cert: export SSL_CERT_FILE=/tmp/azemu-cert.pem
#   3. Set metadata_host: export ARM_METADATA_HOSTNAME=localhost:4567
#
# Then run:
#   terraform init
#   terraform apply -auto-approve
#   terraform destroy -auto-approve

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

  # Point the provider at azemu
  metadata_host = "localhost:4567"

  # Skip provider registration (azemu accepts all registrations, but this
  # avoids unnecessary round-trips during init)
  skip_provider_registration = true

  # Mock credentials (azemu accepts anything)
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
