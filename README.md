# azemu

A local Azure emulator for Terraform-first development. Think LocalStack, but for Azure.

**Status:** v0.1-dev (scaffold, not yet validated against Terraform)

## What it does

azemu runs a local HTTP/HTTPS server that implements enough of the Azure ARM API
surface for `terraform init/apply/destroy` to work without an Azure subscription.
The official `hashicorp/azurerm` provider connects to azemu via its `metadata_host`
configuration, requiring zero provider forks or patches.

## Quick start

```bash
# Build
make build

# Run (HTTP on :4566, HTTPS on :4567)
make run

# Or with Docker
make docker-run
```

azemu prints a cert path on startup. Export it so Terraform trusts the HTTPS endpoint:

```bash
export SSL_CERT_FILE=/tmp/azemu-cert.pem
export ARM_METADATA_HOSTNAME=localhost:4567
```

Then point Terraform at it:

```hcl
provider "azurerm" {
  features {}
  metadata_host              = "localhost:4567"
  skip_provider_registration = true
  subscription_id            = "00000000-0000-0000-0000-000000000000"
  tenant_id                  = "00000000-0000-0000-0000-000000000001"
  client_id                  = "00000000-0000-0000-0000-000000000002"
  client_secret              = "azemu-mock-secret"
}
```

```bash
cd test/terraform
terraform init
terraform apply -auto-approve
```

## How it works

The `azurerm` Terraform provider supports a `metadata_host` field. When set,
the provider fetches cloud environment configuration from
`https://{metadata_host}/metadata/endpoints` instead of using built-in Azure
endpoint profiles. azemu serves this endpoint and returns URLs pointing back
to itself, so all subsequent ARM calls, token requests, and data plane calls
stay local.

## Current support

| Capability | Status |
|---|---|
| Metadata service (`/metadata/endpoints`) | Full |
| OAuth2 token endpoint (mock JWT) | Full |
| OIDC discovery + JWKS | Full |
| Subscriptions / Tenants | Full (mock) |
| Provider registration | Full (always succeeds) |
| Resource Groups (CRUD + HEAD) | Full |
| State export/import | Scaffold |

See [docs/PARITY.md](docs/PARITY.md) for the full compatibility matrix.

## Project structure

```text
CLAUDE.md          -- AI agent instructions (Claude Code)
AGENTS.md          -- subagent skill definitions
TASKS.md           -- implementation plan and task tracking
cmd/azemu/         -- binary entrypoint
internal/          -- core packages (metadata, auth, arm, store, middleware)
pkg/config/        -- public configuration
scripts/           -- aztf wrapper, cert trust helpers
test/terraform/    -- Terraform integration tests
test/integration/  -- Go integration tests
docs/              -- extended documentation
```

## Development

```bash
# Full validation
go vet ./...
go test ./... -v -count=1 -race
make smoke
```

See [TASKS.md](TASKS.md) for the implementation plan and current status.
See [CLAUDE.md](CLAUDE.md) for agent operating rules and coding conventions.

## Roadmap

- [ ] v0.1: Resource groups, Terraform apply/destroy, test coverage, state persistence
- [ ] v0.2: VNets, DNS zones, storage accounts, key vault secrets
- [ ] v0.3: IMDS, workload identity, Azure DevOps OIDC
- [ ] v0.4: Wrapper CLI (aztf), snapshot/fixture system
- [ ] v0.5: Plugin SDK, community resource modules

## Inspired by

- [MiniBlue](https://miniblue.io) for proving the `metadata_host` approach works
- [LocalStack](https://localstack.cloud) for the developer workflow model

## Licence

MIT
