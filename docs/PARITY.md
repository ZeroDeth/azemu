# Parity Matrix -- azemu

What azemu implements. Updated whenever a resource handler changes.

## Infrastructure Services

| Capability | Status | Notes |
|-----------|--------|-------|
| Metadata service (`/metadata/endpoints`) | Full | Supports `azurerm` provider environment discovery |
| OAuth2 token endpoint (mock JWT) | Full | Accepts any credentials, returns RS256 JWT |
| OIDC discovery (`/.well-known/openid-configuration`) | Full | Standard discovery document |
| JWKS (`/discovery/v2.0/keys`) | Full | RSA public key matching signing key |
| Self-signed TLS (ECDSA P-256) | Full | Auto-generated at startup |

## ARM Resources

| Resource | ARM CRUD | Data Plane | Terraform Resource | Status |
|----------|----------|------------|-------------------|--------|
| Subscriptions | Read-only | N/A | N/A | Full (mock) |
| Tenants | Read-only | N/A | N/A | Full (mock) |
| Provider Registration | Always succeeds | N/A | N/A | Full |
| Resource Groups | Full | N/A | `azurerm_resource_group` | Full |
| Virtual Networks | Full | N/A | `azurerm_virtual_network` | Full (inline subnets in PUT body ignored; use `azurerm_subnet`) |
| Subnets | Full | N/A | `azurerm_subnet` | Full (404 `ParentResourceNotFound` if vnet missing; cascades with parent) |
| DNS Zones | None | N/A | `azurerm_dns_zone` | Planned (v0.2) |
| Storage Accounts | None | None | `azurerm_storage_account` | Planned (v0.2) |
| Key Vault Secrets | None | None | `azurerm_key_vault_secret` | Planned (v0.2) |

## Identity

| Capability | Status | Notes |
|-----------|--------|-------|
| Service principal (client_id/secret) | Full | Accepts any credentials |
| Managed identity (IMDS) | None | Planned (v0.3) |
| Workload identity (OIDC federation) | None | Planned (v0.3) |
| Azure DevOps OIDC (`SYSTEM_OIDCREQUESTURI`) | None | Planned (v0.3) |
| ADO Service Connections CRUD | None | Planned (v0.3) |

## Developer Tooling

| Capability | Status | Notes |
|-----------|--------|-------|
| Docker image | Scaffold | Dockerfile exists, not yet tested |
| Binary releases | None | Planned (goreleaser) |
| Wrapper CLI (`aztf`) | None | Planned (v0.1 Phase 3) |
| State export/import | Scaffold | Store interface exists, HTTP endpoints not yet |
| Fixtures/snapshots | None | Planned (v0.2+) |
| `terraform test` examples | None | Planned (v0.1 Phase 3) |
| Helm chart | None | Planned (v0.3+) |

---

Status key:

- **Full**: implemented and tested
- **Scaffold**: code exists but not validated or tested
- **Stub**: endpoint returns 200 but no real logic
- **None**: not implemented
- **Planned**: on roadmap with target version
