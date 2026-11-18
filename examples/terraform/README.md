# azemu examples: Terraform

Run the official `hashicorp/azurerm` provider against a local azemu instance.
No Azure subscription, no account, no network required.

## Prerequisites

- Docker and Docker Compose
- Terraform 1.6+

## Quick start

From the repository root:

```bash
# Start azemu in the background (builds the image on first run).
docker compose up -d --build

# Trust the self-signed cert for this shell session.
export SSL_CERT_FILE=$PWD/.azemu/cert-bundle.pem

# Run Terraform against azemu.
cd examples/terraform
terraform init
terraform apply -auto-approve
terraform destroy -auto-approve
```

When finished:

```bash
cd ../..
docker compose down
```

## What gets created

| Resource | Name | Notes |
|---|---|---|
| Resource Group | `azemu-example-rg` | Location: `uksouth` |
| Virtual Network | `azemu-example-vnet` | Address space: `10.0.0.0/16` |
| Subnet | `azemu-example-subnet` | Address prefix: `10.0.1.0/24` |

## Using azemu tf

`azemu tf` auto-starts the emulator, injects `SSL_CERT_FILE` and `ARM_*`
env vars, and execs terraform:

```bash
azemu tf -chdir=examples/terraform init
azemu tf -chdir=examples/terraform apply -auto-approve
azemu tf -chdir=examples/terraform destroy -auto-approve
```

## Running Terraform tests

Requires Terraform 1.6+ (native test framework):

```bash
export SSL_CERT_FILE=$PWD/.azemu/cert-bundle.pem
cd examples/terraform
terraform test
```

Or from the repo root:

```bash
make tf-test
```

The end-to-end resource scenarios use independent fixtures and lockfiles:

| Provider | Fixtures | Command | CI |
|----------|----------|---------|----|
| azurerm v4.34/v4.35 | [`scenarios/`](./scenarios/) | `make tf-test-scenarios-v4` | Existing suite |
| azurerm v5.14.0 | [`scenarios-v5/`](./scenarios-v5/) | `make tf-test-scenarios-v5` | Local only |

The v4 fixtures keep their original resource configuration. The v5
fixtures use `user_assigned_identity_id` for federated credentials, specify
`rbac_authorization_enabled` for Key Vault, and declare an AKS
`node_provisioning_profile`. The [parity matrix](../../docs/PARITY.md#provider-version-differences)
lists the additional ARM reads observed in v5.

On Linux with Docker host networking, use the container runner if your local
resolver does not resolve `*.vault.localhost`. Start azemu with Docker Compose
first. The aliases apply only inside the Terraform container and cover the
default scenario names, not custom vault names:

```bash
make tf-test-scenarios-v4 TF_SCENARIO_RUNNER=docker
make tf-test-scenarios-v5 TF_SCENARIO_RUNNER=docker
```

## Customisation

Edit `variables.tf` to change the location or resource name prefix.
