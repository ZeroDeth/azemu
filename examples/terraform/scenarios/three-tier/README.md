# Scenario: Three-tier web application

Demonstrates a classic web/app/data architecture using networking resources
against a local azemu instance. No Azure subscription required.

## Resources created

| Resource | Name | Notes |
|---|---|---|
| Resource Group | `azemu-3tier-rg` | Container for all resources |
| Virtual Network | `azemu-3tier-vnet` | `10.0.0.0/16` address space |
| Subnet (web) | `web` | `10.0.1.0/24` |
| Subnet (app) | `app` | `10.0.2.0/24` |
| Subnet (data) | `data` | `10.0.3.0/24` |
| NSG (web) | `azemu-3tier-web-nsg` | Allows HTTP/HTTPS inbound |
| NSG (app) | `azemu-3tier-app-nsg` | Allows port 8080 from web subnet |
| NSG (data) | `azemu-3tier-data-nsg` | Allows port 5432 from app subnet |
| Public IP (LB) | `azemu-3tier-lb-pip` | Static, Standard SKU |
| Public IP (AppGW) | `azemu-3tier-appgw-pip` | Static, Standard SKU |
| Load Balancer | `azemu-3tier-lb` | Standard SKU with HTTP probe and rule |
| Application Gateway | `azemu-3tier-appgw` | Standard_v2 with routing to app tier |

## Prerequisites

- Docker and Docker Compose
- Terraform 1.6+

## Quick start

From the repository root:

```bash
docker compose up -d --build
export SSL_CERT_FILE=$PWD/.azemu/cert-bundle.pem
cd examples/terraform/scenarios/three-tier
terraform init
terraform apply -auto-approve
terraform destroy -auto-approve
```

## Customisation

Override variables on the command line:

```bash
terraform apply -auto-approve \
  -var="prefix=myapp" \
  -var="location=westeurope"
```
