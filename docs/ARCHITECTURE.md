# Architecture -- azemu

How azemu is wired together. For coding conventions see `docs/CONVENTIONS.md`.
For what is implemented see `docs/PARITY.md`.

## Request flow

```text
Developer / CI
    |
    v
Terraform CLI -----> HTTPS :4567 -----> Metadata Service (/metadata/endpoints)
    |                                    Auth Service (OAuth2, OIDC, JWKS)
    |
    +---------------> HTTPS :4566 -----> ARM Facade Router
                                           +-- Subscriptions / Tenants
                                           +-- Provider Registration
                                           +-- Resource Groups (CRUD + cascade)
                                           +-- Virtual Networks (CRUD + child subnets)
                                           +-- Subnets (CRUD, parent-aware)
                                           +-- [v0.2+: DNS, Storage, KeyVault...]
                                         |
                                         v
                                      State Store (in-memory)
                                         |
                                         v
                                      Export/Import (JSON file)
```

Both ports serve HTTPS using the same self-signed ECDSA P-256 certificate.
Port 4566 must be HTTPS because the `azurerm` provider classifies environments
whose `resourceManager` URL uses `http://` as Azure Stack and rejects them.

## How provider redirection works

The `hashicorp/azurerm` Terraform provider has a `metadata_host` field. When
set, the provider calls `https://{metadata_host}/metadata/endpoints` to
discover Azure service URLs instead of using built-in cloud profiles. The
provider code does:

```go
environments.FromEndpoint(ctx, fmt.Sprintf("https://%s", metadataHost))
```

azemu serves this endpoint and returns URLs pointing back to itself, so all
subsequent ARM calls, token requests, and data plane calls stay local.

This requires:

- HTTPS on `:4567` with a self-signed cert (TLS mandatory for metadata).
- HTTPS on `:4566` for ARM and data plane (HTTP triggers `IsAzureStack` rejection).
- A canonical metadata schema matching real Azure verbatim, so `go-azure-sdk`
  can build per-service authorizers without falling through to the Azure Stack
  rejection path. See `internal/metadata/service.go` and its regression tests.
- Mock OAuth2 token endpoint returning valid RS256-signed JWTs.
- Case-insensitive ARM path normalization — azurerm sends camelCase
  `resourceGroups`; chi routes are lowercase. See
  `internal/middleware/pathcase.go`.

## Package layout

```text
cmd/azemu/main.go              entrypoint, server setup, graceful shutdown
internal/
  metadata/service.go          /metadata/endpoints (canonical Azure schema)
  auth/token.go                OAuth2 token endpoint, OIDC discovery, JWKS
  auth/tls.go                  LoadOrGenerateSelfSignedTLS persists via AZEMU_CERT_PATH
  arm/router.go                ARM facade: subscriptions, providers, RG-resources list
  arm/resourcegroup.go         resource group CRUD (cascade delete via store prefix)
  arm/vnet.go                  virtual networks CRUD + HEAD + embedded child subnets
  arm/subnet.go                subnets CRUD + HEAD with parent-vnet existence check
  arm/helpers.go               shared ARM response builders, error formatting
  store/store.go               Store interface definition
  store/memory.go              in-memory implementation
  middleware/azure.go          Azure headers, api-version enforcement
  middleware/pathcase.go       NormalizePath: lowercase canonical ARM literals, collapse `//`
  middleware/logging.go        request/response logging with zerolog
  middleware/unhandled.go      catch-all for unrouted paths (log + 501)
pkg/
  config/config.go             env-based config (HTTPPort/HTTPSPort/CertPath/...)
  armtypes/types.go            shared ARM request/response structs
test/
  terraform/main.tf            azurerm provider config for end-to-end tests
  integration/arm_test.go      in-process httptest CRUD across RG/VNet/Subnet
docs/                          extended documentation
.claude/rules/                 path-scoped rules for coding agents
.flox/env/manifest.toml        pinned dev env (Go, Terraform, pre-commit, ...)
.pre-commit-config.yaml        whitespace + go vet/build + golangci-lint + markdownlint
```

## Dependency direction (enforced)

```text
cmd/azemu ---> internal/* ---> pkg/*
                           ---> store.Store (interface)
```

Inside `internal/`:

- `arm/` may import `store/` and `pkg/armtypes/`
- `metadata/` may import `pkg/config/`
- `middleware/` and `auth/` are standalone
- No cross-imports between sibling internal packages

If you need a shared type across `internal/*` packages, promote it to `pkg/`.

## Dependencies (go.mod)

| Package | Purpose | Pinned |
|---------|---------|--------|
| `go-chi/chi/v5` | HTTP routing | ~v5.1 |
| `golang-jwt/jwt/v5` | JWT creation/validation | ~v5.2 |
| `google/uuid` | Request IDs | ~v1.6 |
| `rs/zerolog` | Structured logging | ~v1.33 |

Standard library for TLS, crypto, testing, flags, JSON. Do not add cobra,
viper, urfave/cli, testify, gomock, or any other framework without approval.
