# Scenario: Redis cache with connection string in Key Vault

Demonstrates `azurerm_redis_cache` together with `azurerm_key_vault` and
`azurerm_key_vault_secret` against a local azemu instance. No Azure
subscription required.

This is a common real-world pattern: provision a managed Redis cache for
application session or state storage, then store its connection string in
Key Vault so the application reads it as a secret instead of embedding it.

## Resources created

| Resource | Name | Notes |
|---|---|---|
| Resource Group | `azemu-redis-rg` | Container for all resources |
| Redis Cache | `azemu-redis-cache` | Standard C1, TLS 1.2, SSL-only |
| Key Vault | `azemu-redis-kv` | Holds the connection string |
| Key Vault Secret | `redis-connection-string` | Value is the cache's primary connection string |

azemu returns deterministic development keys from the Redis `listKeys`
endpoint, so the provider populates `primary_access_key` and
`primary_connection_string`. The cache hostname comes from azemu's
configured Redis endpoint, and the SSL port is `6380`.

## Secret handling note

Passing a value into `azurerm_key_vault_secret` writes that value into the
Terraform state file. Against azemu the connection string contains only a
deterministic development key, so nothing sensitive is stored here. In a real
Azure deployment this pattern makes the state secret-bearing: keep state in an
encrypted remote backend with access controls, or have the cache publish its
own access keys to Key Vault out of band rather than round-tripping them
through Terraform.

## Prerequisites

- Docker and Docker Compose
- Terraform 1.6+ or OpenTofu 1.6+

## Quick start

From the repository root:

```bash
docker compose up -d --build
export SSL_CERT_FILE=$PWD/.azemu/cert-bundle.pem
cd examples/terraform/scenarios/redis-cache
terraform init
terraform apply -auto-approve
terraform destroy -auto-approve
```

OpenTofu works the same way; swap `terraform` for `tofu`.

## Customisation

Override variables on the command line:

```bash
terraform apply -auto-approve \
  -var="prefix=demo" \
  -var="location=westeurope"
```
