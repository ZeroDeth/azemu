# Scenario: Static site with CDN and DNS

Demonstrates a static website hosted on Azure Storage behind a CDN profile
with a custom DNS zone, all running against a local azemu instance. No Azure
subscription required.

## Resources created

| Resource | Name | Notes |
|---|---|---|
| Resource Group | `azemusite-rg` | Container for all resources |
| Storage Account | `azemusitesa` | StorageV2, Standard_LRS |
| Blob Container | `$web` | Public blob access for static content |
| CDN Profile | `azemusite-cdn` | Standard_Microsoft SKU |
| CDN Endpoint | `azemusite-endpoint` | Origin points to storage blob endpoint |
| DNS Zone | `staticsite.local` | Custom domain zone |
| CNAME Record | `www.staticsite.local` | Points to CDN endpoint FQDN |
| TXT Record | `cdnverify.staticsite.local` | CDN domain verification |

## Prerequisites

- Docker and Docker Compose
- Terraform 1.6+

## Quick start

From the repository root:

```bash
docker compose up -d --build
export SSL_CERT_FILE=$PWD/.azemu/cert-bundle.pem
cd examples/terraform/scenarios/static-site
terraform init
terraform apply -auto-approve
terraform destroy -auto-approve
```

## Customisation

Override variables on the command line:

```bash
terraform apply -auto-approve \
  -var="zone_name=mysite.example" \
  -var="prefix=demo"
```
