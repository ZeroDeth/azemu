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

## Using the aztf wrapper

The `scripts/aztf` wrapper automates the `SSL_CERT_FILE` export and starts
azemu via docker-compose if it is not already running:

```bash
./scripts/aztf -chdir=examples/terraform init
./scripts/aztf -chdir=examples/terraform apply -auto-approve
./scripts/aztf -chdir=examples/terraform destroy -auto-approve
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

## Customisation

Edit `variables.tf` to change the location or resource name prefix.
