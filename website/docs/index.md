# azemu

A local Azure emulator for Terraform-first development. Think LocalStack, but for Azure.

## Why azemu

- **No Azure subscription required.** Run `terraform apply` against a local HTTPS server. No login, no cost.
- **No provider forks.** The official `hashicorp/azurerm` provider connects via its built-in `metadata_host` field. Zero patches.
- **Terraform-first fidelity.** Every resource passes a real `terraform apply` + `terraform destroy` round-trip against unmodified `azurerm` v4.x. If it does not round-trip, it is not shipped.

## Quick start

```bash
# Start azemu
docker compose up -d --build

# Trust the self-signed cert for this shell session
export SSL_CERT_FILE=$PWD/.azemu/cert-bundle.pem

# Run Terraform against azemu
cd examples/terraform
terraform init
terraform apply -auto-approve
terraform destroy -auto-approve
```

See [Your First Apply](getting-started/first-apply.md) for a full step-by-step walkthrough with expected output.

## Current status

azemu supports the following Azure resource types:

- Resource Groups
- Virtual Networks and Subnets
- Public IP Addresses
- Network Security Groups
- Load Balancers
- Application Gateways
- DNS Zones
- Storage Accounts
- Key Vault (management plane)
- CDN Profiles and Endpoints
- Managed Identities (user-assigned)
- AKS (management plane)
- Azure DevOps OIDC and service connections

See the [Parity Matrix](concepts/parity-matrix.md) for the full implementation status of each resource type.
