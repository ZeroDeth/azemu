---
---

<div class="azemu-hero" markdown>

# azemu

<p class="azemu-tagline">An open-source local Azure emulator for Terraform and OpenTofu<span class="azemu-cursor"></span></p>

<p class="azemu-badges" markdown>
[![License: MIT](https://img.shields.io/badge/License-MIT-00ff41.svg)](community/license.md)
[![PRs welcome](https://img.shields.io/badge/PRs-welcome-00ff41.svg)](community/contributing.md)
[![Works with OpenTofu](https://img.shields.io/badge/OpenTofu-compatible-00ff41.svg)](community/license.md)
[![GitHub](https://img.shields.io/badge/GitHub-zerodeth%2Fazemu-58a6ff.svg)](https://github.com/zerodeth/azemu)
</p>

<div class="azemu-cta-group">
<a href="getting-started/install/" class="azemu-cta">Get Started</a>
<a href="https://github.com/zerodeth/azemu" class="azemu-cta azemu-cta--secondary">View on GitHub</a>
<a href="community/contributing/" class="azemu-cta azemu-cta--secondary">Contribute</a>
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

<div class="azemu-feature" markdown>

[**Web Console**{ .azemu-feature-title }](concepts/web-console.md)

A local Azure Portal for your emulator. Three views, live request streaming, embedded in the binary.

</div>

</div>

## Quick start

```bash
# Start azemu (Azure ARM emulator + Azurite storage sidecar)
docker compose up -d --build

# Trust the self-signed cert for this shell
export SSL_CERT_FILE=$PWD/.azemu/cert-bundle.pem

# Run Terraform against the example stack
terraform -chdir=examples/terraform init
terraform -chdir=examples/terraform apply -auto-approve
```

azemu serves the standard Azure endpoints, so the same `azurerm` provider runs
unmodified under both Terraform and [OpenTofu](https://opentofu.org). Swap
`terraform` for `tofu` and everything works the same.

!!! info "Prerequisites"
    Requires Docker, Docker Compose, and Terraform 1.6+ or OpenTofu 1.6+.
    See [Your First Apply](getting-started/first-apply.md) for the full
    walkthrough with expected output.

## What azemu emulates today

Everything below round-trips a real `terraform apply` and `destroy` against
the unmodified `azurerm` provider. No subscription, no account, no network.

| Area | Resources |
|------|-----------|
| **Networking** | Resource Groups, Virtual Networks, Subnets, Public IPs, NSGs, Load Balancers, Application Gateways, DNS Zones (with record sets) |
| **Storage & secrets** | Storage Accounts + Blob containers (data plane via Azurite), Key Vault secrets and keys (including `sign`) |
| **Compute & identity** | User-Assigned Managed Identities, Federated Identity Credentials, AKS (management plane) |
| **CDN & CI/CD** | CDN Profiles and Endpoints (with a content data plane), Redis Cache, Azure DevOps OIDC + service connections |

See the [Parity Matrix](concepts/parity-matrix.md) for the per-resource
support depth, each row linked to the test that proves it.

## Examples that solve real use cases

Each scenario is a full, working stack you can `apply` and `destroy` locally.
They double as integration tests, so they keep working.

- **[Three-tier web app](https://github.com/zerodeth/azemu/tree/main/examples/terraform/scenarios/three-tier)**: a web/app/data architecture built from networking resources.
- **[Static site with CDN and DNS](https://github.com/zerodeth/azemu/tree/main/examples/terraform/scenarios/static-site)**: a static website on Storage behind a CDN profile and a custom DNS zone.
- **[DNS zone with records](https://github.com/zerodeth/azemu/tree/main/examples/terraform/scenarios/dns-with-records)**: a DNS zone with A, AAAA, CNAME, TXT, and MX record sets.
- **[Redis cache with Key Vault](https://github.com/zerodeth/azemu/tree/main/examples/terraform/scenarios/redis-cache)**: a Redis cache whose connection string is stored in Key Vault, the read-from-secret pattern.
- **[AKS workload](https://github.com/zerodeth/azemu/tree/main/examples/terraform/scenarios/aks-workload)**: management-plane provisioning for a workload that reads Key Vault secrets via workload identity.
- **[Azure DevOps pipeline](https://github.com/zerodeth/azemu/tree/main/examples/terraform/scenarios/ado-pipeline)**: the ARM resources for an ADO pipeline that authenticates with workload identity federation (OIDC).
- **[Server-less OTA delivery](https://github.com/zerodeth/azemu/tree/main/examples/terraform/scenarios/ota-delivery)**: an end-to-end over-the-air update flow with a Key Vault-signed manifest, immutable Blob artefacts, and a CDN read path, verified locally with no compute on the read path.

## What we are aiming for

Today azemu solves the resources and scenarios above end to end. The aim is
to cover more of Azure so more teams can test their stack locally, and to
keep raising fidelity on what already exists. Coverage grows from real
demand, so the most useful thing you can do is tell us what you need:

- Open a [feature request](https://github.com/zerodeth/azemu/issues/new?labels=enhancement)
  naming the `azurerm` resource and the ARM provider path, or
- Start a [discussion](https://github.com/zerodeth/azemu/discussions) with
  your use case.

## Get involved

azemu is MIT-licensed, with no paid tier, no telemetry, and no account to
create. If it is useful to you, a
[GitHub star](https://github.com/zerodeth/azemu) helps other people find it.

Contributions of every size are welcome, and you do not need to be an Azure
or Go expert. Adding a resource, fixing a rejected `apply`, or sharpening a
doc page all help.

<div class="azemu-cta-group">
<a href="community/contributing/" class="azemu-cta">Start Contributing</a>
<a href="https://github.com/zerodeth/azemu" class="azemu-cta azemu-cta--secondary">Star on GitHub</a>
</div>
