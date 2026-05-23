# azemu

A local Azure emulator for Terraform-first development. Think LocalStack, but for Azure.

**Status:** v0.1-dev. `terraform init && apply && destroy` proven end-to-end against the
official `hashicorp/azurerm` v4.x provider (resource groups, virtual networks, subnets).

See [ROADMAP.md](ROADMAP.md) for the vision, resource roster, and milestones.

## What it does

azemu runs a local HTTPS server that implements enough of the Azure ARM API
surface for `terraform init/apply/destroy` to work without an Azure subscription.
The official `hashicorp/azurerm` provider connects to azemu via its `metadata_host`
configuration, requiring zero provider forks or patches.

## Quick start (Docker, recommended)

Requires Docker, Docker Compose, and Terraform 1.6+.

```bash
# Start azemu.
docker compose up -d --build

# Trust the self-signed cert for this shell session.
export SSL_CERT_FILE=$PWD/.azemu/cert-bundle.pem

# Run Terraform against azemu.
cd examples/terraform
terraform init
terraform apply -auto-approve
terraform destroy -auto-approve

# Clean up.
cd ../..
docker compose down
```

Or use `azemu tf` which auto-starts the emulator, injects all env vars,
and execs terraform:

```bash
azemu tf -chdir=examples/terraform init
azemu tf -chdir=examples/terraform apply -auto-approve
azemu tf -chdir=examples/terraform destroy -auto-approve
```

See [examples/terraform/README.md](examples/terraform/README.md) for details.

## Quick start (flox, contributor workflow)

The repo ships a flox environment with Go, Terraform, pre-commit, and helper
functions pre-pinned. This is the contributor-side workflow; Docker is what
new users hit first.

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

See [docs/SETUP.md](docs/SETUP.md) for the full contributor guide.

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
| Health check (`GET /health` on `:4568`) | Full |
| State export/import (file-backed) | Full |

See [docs/PARITY.md](docs/PARITY.md) for the full compatibility matrix.

## Ports

| Port | Protocol | Purpose |
|---|---|---|
| 4566 | HTTPS | ARM API |
| 4567 | HTTPS | Metadata, OAuth2, OIDC |
| 4568 | HTTP | Health check (container probes, no TLS) |

## Project structure

```text
CLAUDE.md               -- AI agent instructions (Claude Code)
AGENTS.md               -- subagent skill definitions
TASKS.md                -- phased implementation plan
TODO.md                 -- known gaps and post-mortems
ROADMAP.md              -- vision, milestones, resource roster
.flox/                  -- pinned dev environment (Go, Terraform, pre-commit, ...)
.pre-commit-config.yaml -- hygiene + go vet/build + golangci-lint + markdownlint
Makefile                -- build, test, smoke, docker, coverage targets
Dockerfile              -- multi-stage Go build
docker-compose.yml      -- one-command local setup with healthcheck
flake.nix               -- Nix flake for upstream Nix users
scripts/trust-cert.sh   -- optional: add cert to system keychain (macOS/Linux)
cmd/azemu/              -- binary entrypoint
internal/               -- core packages (metadata, auth, arm, store, middleware)
pkg/config/             -- public configuration
examples/terraform/     -- docker-compose quick-start example (RG + VNet + Subnet)
test/terraform/         -- flox-based integration fixture
test/integration/       -- Go integration tests
docs/                   -- SETUP, TROUBLESHOOTING, PARITY, ARCHITECTURE
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
See [docs/SETUP.md](docs/SETUP.md) for the contributor onboarding guide.

## Roadmap

- [x] v0.1 Phase 1: Terraform apply/destroy round-trip against azurerm v4.x
- [x] v0.1 Phase 2: Test coverage backfill (store, auth, middleware, config)
- [x] v0.1 Phase 3: Docker, docker-compose, examples, startup banner
- [x] v0.1 Phase 4: File-backed state, export/import HTTP endpoints
- [ ] v0.1 Phase 5: Docs, governance, release prep (in progress)
- [ ] v0.2: DNS zones, storage accounts, key vault secrets
- [ ] v0.3: IMDS, workload identity, Azure DevOps OIDC

See [ROADMAP.md](ROADMAP.md) for the full resource roster and milestones.

## Inspired by

- [MiniBlue](https://miniblue.io) for proving the `metadata_host` approach works
- [LocalStack](https://localstack.cloud) for the developer workflow model

## Licence

MIT
