# Your First Apply

A step-by-step guide to running `terraform apply` against azemu.

## Prerequisites

- Docker and Docker Compose
- Terraform 1.6+

Verify both are installed:

```bash
docker --version
terraform version
```

## Step 1: Start azemu

From the repository root:

```bash
docker compose up -d --build
```

This starts two containers:

- **azemu**: the ARM emulator. It serves the Azure Resource Manager REST API on
  port 4566 (HTTPS) and the health endpoint on port 4568.
- **Azurite**: Microsoft's official Azure Storage emulator, used as a sidecar for
  Storage data-plane operations.

The first build takes a minute or two while Go compiles the binary. Subsequent
starts reuse the image and are faster.

Verify both containers are running:

```bash
docker compose ps
```

You should see two entries, both in `running` state.

## Step 2: Trust the certificate

azemu serves HTTPS using a self-signed certificate generated at first start.
Terraform's HTTP client requires a trusted CA, so you need to point it at the
cert bundle before running any `terraform` commands.

```bash
export SSL_CERT_FILE=$PWD/.azemu/cert-bundle.pem
```

This export applies to the current shell session only. If you open a new
terminal, re-run the export from the repository root before running Terraform
again.

For persistent trust (so you do not need to re-export after every terminal
restart), see [Troubleshooting](../resources/troubleshooting.md).

## Step 3: Initialize Terraform

```bash
cd examples/terraform
terraform init
```

Terraform downloads the official `hashicorp/azurerm` provider and reads the
`metadata_host` value from `examples/terraform/main.tf`. When `metadata_host`
is set, the provider fetches cloud endpoint configuration from azemu instead
of using its built-in Azure endpoint list. All subsequent ARM calls, token
requests, and data-plane calls stay local.

A successful `terraform init` prints something like:

```text
Terraform has been successfully initialized!
```

## Step 4: Apply

```bash
terraform apply -auto-approve
```

Terraform creates three resources against azemu:

- A resource group (`azemu-example-rg` in `uksouth`)
- A virtual network (`azemu-example-vnet` with address space `10.0.0.0/16`)
- A subnet (`azemu-example-subnet` with prefix `10.0.1.0/24`)

azemu processes each `PUT` request, stores the resource in its in-memory state,
and returns a response that matches the Azure ARM API contract. Terraform marks
the resources as created and writes them to `terraform.tfstate`.

A successful apply ends with:

```text
Apply complete! Resources: 3 added, 0 changed, 0 destroyed.
```

## Step 5: Verify

Confirm the resource group exists in azemu's state by querying the ARM API
directly:

```bash
curl -sk https://127.0.0.1:4566/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups?api-version=2022-12-01 | jq .
```

The `-s` flag suppresses curl's progress output. The `-k` flag skips TLS
verification (the `SSL_CERT_FILE` approach works too; `-k` is simpler for
one-off checks). The response is a JSON list of resource groups with their
properties and provisioning state.

## Step 6: Destroy

```bash
terraform destroy -auto-approve
```

Terraform sends `DELETE` requests for each resource in dependency order
(subnet first, then virtual network, then resource group). azemu removes them
from state and returns `202 Accepted` responses. Terraform waits for the
async deletion to complete and then removes the entries from `terraform.tfstate`.

A successful destroy ends with:

```text
Destroy complete! Resources: 3 destroyed.
```

## Step 7: Clean up

```bash
cd ../..
docker compose down
```

`docker compose down` stops and removes the azemu and Azurite containers. The
cert bundle at `.azemu/cert-bundle.pem` is preserved on disk, so the next
`docker compose up` reuses it and skips cert generation.

## Next steps

- [Parity Matrix](../concepts/parity-matrix.md): What else azemu supports
- [Architecture](../concepts/architecture.md): How the metadata-redirect pattern works
- [Contributing](../community/contributing.md): Add a new ARM resource
