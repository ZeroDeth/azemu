# azemu

An open-source local Azure emulator for Terraform and OpenTofu. Think
LocalStack, but for Azure.

[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![PRs welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)
[![Works with OpenTofu](https://img.shields.io/badge/OpenTofu-compatible-brightgreen.svg)](https://opentofu.org)
[![Docs](https://img.shields.io/badge/docs-zerodeth.github.io%2Fazemu-blue.svg)](https://zerodeth.github.io/azemu)

**Status:** v0.3.0. `terraform init && terraform apply && terraform destroy` proven
end-to-end against the official `hashicorp/azurerm` v4.x provider across networking,
storage, Key Vault, CDN, identity, AKS, and Azure DevOps OIDC. The same loop works
with [OpenTofu](https://opentofu.org), a drop-in replacement.

See [ROADMAP.md](ROADMAP.md) for the vision, resource roster, and milestones.
**Contributions are welcome,** see [CONTRIBUTING.md](CONTRIBUTING.md) and the
[good first issues](https://github.com/zerodeth/azemu/labels/good%20first%20issue).

## What it does

azemu runs a local HTTPS server that implements enough of the Azure ARM API
surface for `terraform init/apply/destroy` to work without an Azure subscription.
The official `hashicorp/azurerm` provider connects to azemu via its `metadata_host`
configuration, requiring zero provider forks or patches. Because the same `azurerm`
provider works with Terraform or OpenTofu, azemu behaves the same under either tool.

The emulated surface is plain Azure (the ARM REST API, the metadata service, and
OIDC), so any tool that speaks Azure can target it. The `azemu` binary ships
subcommands that auto-start the emulator and drive your toolchain against it:
`azemu tf`, `azemu pulumi`, `azemu kubectl`, and `azemu python`. Terraform and
OpenTofu are the proven, tested path today.

## Tell us what you need

azemu grows from real use cases. The [parity matrix](docs/PARITY.md) is the
live map of what is covered. If a resource or behaviour you need is missing,
that is the most useful thing you can tell us:

- Open a [feature request](https://github.com/zerodeth/azemu/issues/new?labels=enhancement)
  naming the `azurerm` resource and the ARM provider path, or
- Start a [discussion](https://github.com/zerodeth/azemu/discussions) with
  your use case.

Coverage expands one tested resource at a time; every Full resource
round-trips a real provider. See [ROADMAP.md](ROADMAP.md).

## Quick start (Docker, recommended)

Requires Docker, Docker Compose, and Terraform 1.6+ or OpenTofu 1.6+.

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
| Web console (`:4570`) | Full |

See [docs/PARITY.md](docs/PARITY.md) for the full compatibility matrix.

## Ports

| Port | Protocol | Purpose |
|---|---|---|
| 4566 | HTTPS | ARM API |
| 4567 | HTTPS | Metadata, OAuth2, OIDC |
| 4568 | HTTP | Health check (container probes, no TLS) |
| 4569 | HTTP | ADO OIDC, service connections |
| 4570 | HTTP | Web console |

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
- [x] v0.1 Phase 5: Docs, governance, release prep
- [x] v0.2: DNS, Load Balancer, App Gateway, Public IP, NSG, Storage, Key Vault secrets+keys, CDN
- [x] v0.3: IMDS, workload identity, Azure DevOps OIDC, AKS, Managed Identity, Redis, CLI subcommands

See [ROADMAP.md](ROADMAP.md) for the full resource roster and milestones.

## Contributing

azemu is built in the open and contributions of every size are welcome, from
a typo fix to a new resource handler. You do not need to be an Azure or Go
expert: a clear bug report of a `terraform apply` or `tofu apply` that azemu
rejects is one of the most useful things you can send.

- Read [CONTRIBUTING.md](CONTRIBUTING.md) for the workflow and PR checklist.
- Browse [good first issues](https://github.com/zerodeth/azemu/labels/good%20first%20issue).
- Ask questions in [Discussions](https://github.com/zerodeth/azemu/discussions).

Full docs live at [zerodeth.github.io/azemu](https://zerodeth.github.io/azemu).

## Inspired by

- [MiniBlue](https://miniblue.io) for proving the `metadata_host` approach works
- [LocalStack](https://localstack.cloud) for the developer workflow model

## Licence

azemu is released under the [MIT License](LICENSE). You are free to use,
modify, fork, and redistribute it, including commercially, as long as the
copyright and licence notice are kept.

If you fork azemu, please rename and rebrand your copy, keep the original MIT
notice, and add your own copyright line. If you publish your own Terraform or
OpenTofu provider from a fork, register it under your own registry namespace
(not `hashicorp/`, `opentofu/`, or `zerodeth/`). azemu itself does not fork
the `azurerm` provider; it uses the official one unmodified via `metadata_host`.

We recommend [OpenTofu](https://opentofu.org) (MPL 2.0) for a fully
open-source toolchain. Terraform 1.6+ is under the source-available BUSL 1.1
licence; azemu works with both. See the
[License & Forking guide](https://zerodeth.github.io/azemu/community/license/)
for the details.
