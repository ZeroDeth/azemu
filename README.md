# azemu

A local Azure emulator for Terraform-first development. Think LocalStack, but for Azure.

**Status:** v0.1-dev. `terraform init && apply && destroy` proven end-to-end against the
official `hashicorp/azurerm` v4.x provider (resource groups, virtual networks, subnets).

## What it does

azemu runs a local HTTPS server that implements enough of the Azure ARM API
surface for `terraform init/apply/destroy` to work without an Azure subscription.
The official `hashicorp/azurerm` provider connects to azemu via its `metadata_host`
configuration, requiring zero provider forks or patches.

## Quick start (flox, recommended)

The repo ships a flox environment with Go, Terraform, pre-commit, jq, tflint and
shellcheck pre-pinned, plus helper functions for the common loop.

```bash
flox activate         # installs pre-commit hook on first run
azemu-start           # builds, starts azemu, prints one-time cert-trust command
ta                    # alias: terraform apply -auto-approve in test/terraform
td                    # alias: terraform destroy -auto-approve
azemu-stop
```

`azemu-start` walks you through a one-time `security add-trusted-cert` command
on macOS. The cert is persisted at `.azemu/cert-bundle.pem` (gitignored), so
later restarts reuse it and the keychain prompt does not return.

## Quick start (manual)

```bash
make build
mkdir -p .azemu
AZEMU_CERT_PATH=$PWD/.azemu/cert-bundle.pem ./bin/azemu
```

Then in another shell:

```bash
export ARM_METADATA_HOSTNAME=127.0.0.1:4567   # 127.0.0.1, not localhost — see below
export ARM_SUBSCRIPTION_ID=00000000-0000-0000-0000-000000000000
export ARM_TENANT_ID=00000000-0000-0000-0000-000000000001
export ARM_CLIENT_ID=00000000-0000-0000-0000-000000000002
export ARM_CLIENT_SECRET=azemu-mock-secret
cd test/terraform
terraform init && terraform apply -auto-approve
```

Provider block in `test/terraform/main.tf`:

```hcl
provider "azurerm" {
  features {}
  metadata_host                   = "127.0.0.1:4567"
  resource_provider_registrations = "none"
  subscription_id                 = "00000000-0000-0000-0000-000000000000"
  tenant_id                       = "00000000-0000-0000-0000-000000000001"
  client_id                       = "00000000-0000-0000-0000-000000000002"
  client_secret                   = "azemu-mock-secret"
}
```

> Use `127.0.0.1`, not `localhost`. macOS resolves `localhost` to `::1` first
> and azemu listens on IPv4. `skip_provider_registration` is deprecated in
> azurerm v4.x and silently ignored — use `resource_provider_registrations = "none"`.

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
| Virtual Networks (CRUD + HEAD) | Full |
| Subnets (CRUD + HEAD, parent-aware) | Full |
| State export/import | Scaffold |

See [docs/PARITY.md](docs/PARITY.md) for the full compatibility matrix.

## Project structure

```text
CLAUDE.md               -- AI agent instructions (Claude Code)
AGENTS.md               -- subagent skill definitions
TASKS.md                -- phased implementation plan
TODO.md                 -- known gaps and post-mortems
CHANGELOG.md            -- keep-a-changelog history
.flox/                  -- pinned dev environment (Go, Terraform, pre-commit, ...)
.pre-commit-config.yaml -- hygiene + go vet/build + golangci-lint + markdownlint
Makefile                -- build, run, test, smoke, docker targets
cmd/azemu/              -- binary entrypoint
internal/               -- core packages (metadata, auth, arm, store, middleware)
pkg/config/             -- public configuration
test/terraform/         -- Terraform integration fixtures
test/integration/       -- Go integration tests
docs/                   -- SETUP, TROUBLESHOOTING, PARITY
```

## Development

```bash
# Full validation
go vet ./...
go test ./... -v -count=1 -race
make smoke
pre-commit run --all-files   # auto-installed by `flox activate`
```

See [TASKS.md](TASKS.md) for the implementation plan and current status.
See [CLAUDE.md](CLAUDE.md) for agent operating rules and coding conventions.
See [docs/SETUP.md](docs/SETUP.md) for the long-form contributor onboarding guide.

## Roadmap

- [x] v0.1 Phase 1: Terraform apply/destroy round-trip against azurerm v4.x
- [ ] v0.1 Phase 2: Test coverage backfill (store, auth, middleware, config)
- [ ] v0.1 Phase 3: aztf wrapper, terraform test, startup banner
- [ ] v0.1 Phase 4: File-backed state, export/import HTTP endpoints
- [ ] v0.2: DNS zones, storage accounts, key vault secrets
- [ ] v0.3: IMDS, workload identity, Azure DevOps OIDC
- [ ] v0.4: Wrapper CLI (aztf), snapshot/fixture system
- [ ] v0.5: Plugin SDK, community resource modules

## Inspired by

- [MiniBlue](https://miniblue.io) for proving the `metadata_host` approach works
- [LocalStack](https://localstack.cloud) for the developer workflow model

## Licence

MIT
