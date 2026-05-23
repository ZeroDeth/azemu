# Scenario: ADO pipeline with workload identity federation

Demonstrates the Azure-side ARM resources needed for an Azure DevOps
pipeline that authenticates via workload identity (OIDC), all running
against a local azemu instance. No Azure subscription required.

The ADO service connection itself is created via the ADO REST API on
azemu's `:4569` endpoint, not via Terraform. This scenario provisions
only the ARM resources that the service connection references.

## Resources created

| Resource | Name | Notes |
|---|---|---|
| Resource Group | `azemu-ado-rg` | Container for all resources |
| User-Assigned Identity | `azemu-ado-pipeline-id` | For workload identity federation |
| Federated Identity Credential | `ado-federation` | OIDC trust for ADO issuer |
| Key Vault | `azemu-ado-kv` | Pipeline secrets |
| Key Vault Secret | `deploy-token` | Sample deployment token |
| Storage Account | `azemuadoartifacts` | Pipeline artifact storage |
| Blob Container | `pipeline-artifacts` | Private container for build artifacts |

## Prerequisites

- Docker and Docker Compose
- Terraform 1.6+

## Quick start

From the repository root:

```bash
docker compose up -d --build
export SSL_CERT_FILE=$PWD/.azemu/cert-bundle.pem
cd examples/terraform/scenarios/ado-pipeline
terraform init
terraform apply -auto-approve
terraform destroy -auto-approve
```

## ADO service connection (optional)

After `terraform apply`, create the ADO service connection via curl:

```bash
curl -s -X POST http://127.0.0.1:4569/azemu-org/azemu-project/_apis/serviceendpoint/endpoints \
  -H "Content-Type: application/json" \
  -d '{
    "name": "azemu-service-connection",
    "type": "AzureRM",
    "data": {
      "subscriptionId": "00000000-0000-0000-0000-000000000000",
      "subscriptionName": "azemu-local"
    }
  }'
```

## Customisation

Override variables on the command line:

```bash
terraform apply -auto-approve \
  -var="prefix=myado" \
  -var="ado_org_url=http://127.0.0.1:4569"
```
