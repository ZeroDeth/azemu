# Scenario: AKS workload with managed identity and Key Vault

Demonstrates the management-plane provisioning for a Kubernetes workload
that reads secrets from Key Vault via workload identity, all running against
a local azemu instance. No Azure subscription required.

## Resources created

| Resource | Name | Notes |
|---|---|---|
| Resource Group | `azemu-aks-rg` | Container for all resources |
| Virtual Network | `azemu-aks-vnet` | `10.1.0.0/16` address space |
| Subnet | `aks-nodes` | `10.1.0.0/22` (1022 IPs for AKS nodes) |
| AKS Cluster | `azemu-aks-cluster` | 3-node default pool, SystemAssigned identity |
| User-Assigned Identity | `azemu-aks-workload-id` | For workload identity federation |
| Key Vault | `azemu-aks-kv` | Standard SKU |
| Key Vault Secret | `app-connection-string` | Sample connection string |

azemu returns a management-plane stub for AKS. The cluster is not a real
Kubernetes API server; it provides the ARM resource shape that Terraform
expects for plan/apply/destroy cycles.

## Prerequisites

- Docker and Docker Compose
- Terraform 1.6+

## Quick start

From the repository root:

```bash
docker compose up -d --build
export SSL_CERT_FILE=$PWD/.azemu/cert-bundle.pem
cd examples/terraform/scenarios/aks-workload
terraform init
terraform apply -auto-approve
terraform destroy -auto-approve
```

## Customisation

Override variables on the command line:

```bash
terraform apply -auto-approve \
  -var="prefix=myaks" \
  -var="kubernetes_version=1.30.0"
```
