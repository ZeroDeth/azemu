---
hide:
  - navigation
  - toc
---

<div class="azemu-hero" markdown>

# azemu

<p class="azemu-tagline">A local Azure emulator for Terraform-first development<span class="azemu-cursor"></span></p>

</div>

<div class="azemu-features" markdown>

<div class="azemu-feature" markdown>

**No subscription**{ .azemu-feature-title }

Run `terraform apply` against a local HTTPS server. No Azure login, no cost, no cloud account.

</div>

<div class="azemu-feature" markdown>

**No provider forks**{ .azemu-feature-title }

The official `hashicorp/azurerm` provider connects via its built-in `metadata_host` field. Zero patches, zero forks.

</div>

<div class="azemu-feature" markdown>

**Terraform-first fidelity**{ .azemu-feature-title }

Every resource passes a real `terraform apply` + `terraform destroy` round-trip against unmodified `azurerm` v4.x.

</div>

</div>

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

<ul class="azemu-status" markdown>

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

</ul>

See the [Parity Matrix](concepts/parity-matrix.md) for the full implementation status of each resource type.
