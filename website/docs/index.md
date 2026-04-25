---
---

<div class="azemu-hero" markdown>

# azemu

<p class="azemu-tagline">A local Azure emulator for Terraform-first development<span class="azemu-cursor"></span></p>

<div class="azemu-cta-group">
<a href="getting-started/install/" class="azemu-cta">Get Started</a>
<a href="https://github.com/zerodeth/azemu" class="azemu-cta azemu-cta--secondary">View on GitHub</a>
</div>

</div>

<div class="azemu-features" markdown>

<div class="azemu-feature" markdown>

[**Getting Started**{ .azemu-feature-title }](getting-started/install.md)

Install azemu and run your first `terraform apply` in minutes. No Azure account needed.

</div>

<div class="azemu-feature" markdown>

[**How It Works**{ .azemu-feature-title }](getting-started/how-it-works.md)

The metadata-redirect pattern, ARM fidelity, and what gets emulated under the hood.

</div>

<div class="azemu-feature" markdown>

[**Parity Matrix**{ .azemu-feature-title }](concepts/parity-matrix.md)

Which Azure resources are supported and at what depth. Full transparency, no surprises.

</div>

</div>

## Quick start

```bash
docker compose up -d --build
./scripts/aztf -chdir=examples/terraform apply -auto-approve
```

!!! info "Prerequisites"
    Requires Docker, Docker Compose, and Terraform 1.6+.
    See [Your First Apply](getting-started/first-apply.md) for the full
    walkthrough with expected output.

## Supported resources

**Networking:** Resource Groups, Virtual Networks, Subnets, Public IPs,
NSGs, Load Balancers, Application Gateways, DNS Zones

**Storage and secrets:** Storage Accounts (data plane via Azurite),
Key Vault (management plane)

**Compute and identity:** Managed Identities (user-assigned),
AKS (management plane)

**CI/CD integration:** CDN Profiles and Endpoints, Azure DevOps
service connections

See the [Parity Matrix](concepts/parity-matrix.md) for the full
resource support matrix with implementation depth per resource type.
