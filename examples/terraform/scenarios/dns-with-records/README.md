# Scenario: DNS zone with record sets

Demonstrates `azurerm_dns_zone` with five record types (A, AAAA, CNAME, TXT, MX)
against a local azemu instance. No Azure subscription required.

## Resources created

| Resource | Name | Notes |
|---|---|---|
| Resource Group | `azemu-dns-rg` | Container for all DNS resources |
| DNS Zone | `example.internal` | Auto-generates SOA and NS records |
| A record | `app.example.internal` | Two IPv4 addresses |
| AAAA record | `app.example.internal` | Two IPv6 addresses |
| CNAME record | `api.example.internal` | Alias to `app.example.internal` |
| TXT record | `@.example.internal` | SPF policy |
| MX record | `@.example.internal` | Two mail exchangers |

azemu assigns four Azure DNS name servers to every zone and computes FQDNs
for all record sets.

## Prerequisites

- Docker and Docker Compose
- Terraform 1.6+

## Quick start

From the repository root:

```bash
docker compose up -d --build
export SSL_CERT_FILE=$PWD/.azemu/cert-bundle.pem
cd examples/terraform/scenarios/dns-with-records
terraform init
terraform apply -auto-approve
terraform destroy -auto-approve
```

## Customisation

Override variables on the command line:

```bash
terraform apply -auto-approve \
  -var="zone_name=myapp.local" \
  -var="prefix=demo"
```
