# Parity Matrix -- azemu

What azemu implements. Updated whenever a resource handler changes.

Every "Full" claim links to the test file that proves it. If a row says
Full, a reviewer should be able to open the linked test and see a passing
assertion for the behaviour. If the link is missing, the row is aspirational
and should read Scaffold or Planned instead.

## Infrastructure Services

| Capability | Status | Proof | Notes |
|-----------|--------|-------|-------|
| Metadata service (`/metadata/endpoints`) | Full | [service_test.go](../internal/metadata/service_test.go), [metadata_test.go](../test/integration/metadata_test.go) | Canonical Azure schema; pinned by `TestMetadata_CanonicalFieldNames`, `TestMetadata_NotClassifiedAsAzureStack` |
| OAuth2 token endpoint (mock JWT) | Full | [token_test.go](../internal/auth/token_test.go), [auth_test.go](../test/integration/auth_test.go) | RS256 JWT, end-to-end JWKS signature verification in the integration suite |
| OIDC discovery (`/.well-known/openid-configuration`) | Full | [token_test.go](../internal/auth/token_test.go) | Standard discovery document; mounted under `/{tenantID}` via `TokenService.TenantRoutes` |
| JWKS (`/discovery/v2.0/keys`) | Full | [token_test.go](../internal/auth/token_test.go), [auth_test.go](../test/integration/auth_test.go) | RSA public key matches signing key; `kid` in token header matches JWKS key |
| Self-signed TLS (ECDSA P-256) | Full | [tls_test.go](../internal/auth/tls_test.go) | Auto-generated at startup; persistent across restarts when `AZEMU_CERT_PATH` is set |
| Health check (`GET /health` on `:4568`) | Full | covered by `make smoke` | Plain HTTP, no TLS, no middleware; returns `{"status":"ok","version":"...","uptime_seconds":N}` |

## ARM Resources

| Resource | ARM CRUD | Data Plane | Terraform Resource | Status | Proof |
|----------|----------|------------|-------------------|--------|-------|
| Subscriptions | Read-only | N/A | N/A | Full (mock) | [rg_test.go](../internal/arm/rg_test.go) (indirect: every RG path is scoped under a subscription) |
| Tenants | Read-only | N/A | N/A | Full (mock) | [token_test.go](../internal/auth/token_test.go) (tenant-scoped routes) |
| Provider Registration | Always succeeds | N/A | N/A | Full | covered by the Terraform round-trip in `make smoke` |
| Resource Groups | Full | N/A | `azurerm_resource_group` | Full | [rg_test.go](../internal/arm/rg_test.go), [rg_resources_test.go](../internal/arm/rg_resources_test.go), [arm_test.go](../test/integration/arm_test.go) |
| Virtual Networks | Full | N/A | `azurerm_virtual_network` | Full (invalid/overlapping `addressPrefixes` rejected with 400; inline subnets in PUT body are dropped — use `azurerm_subnet` instead) | [vnet_test.go](../internal/arm/vnet_test.go), [arm_test.go](../test/integration/arm_test.go) |
| Subnets | Full | N/A | `azurerm_subnet` | Full (404 `ParentResourceNotFound` if vnet missing; cascades with parent) | [subnet_test.go](../internal/arm/subnet_test.go), [arm_test.go](../test/integration/arm_test.go) |
| Public IP Addresses | Full | N/A | `azurerm_public_ip` | Full (Static/Dynamic alloc, SKU, fake `ipAddress` assigned on creation, preserved on update) | [public_ip_test.go](../internal/arm/public_ip_test.go), [arm_test.go](../test/integration/arm_test.go) |
| Network Security Groups | Full | N/A | `azurerm_network_security_group` | Full (security rules as child resources, cascade delete, embedded in NSG GET) | [nsg_test.go](../internal/arm/nsg_test.go), [arm_test.go](../test/integration/arm_test.go) |
| Load Balancers | Full | N/A | `azurerm_lb`, `azurerm_lb_backend_address_pool`, `azurerm_lb_rule`, `azurerm_lb_probe` | Full (backend pools, rules, probes as child resources; cascade delete; embedded in LB GET; SKU at top level) | [lb_test.go](../internal/arm/lb_test.go), [arm_test.go](../test/integration/arm_test.go) |
| Application Gateways | Full | N/A | `azurerm_application_gateway` | Full (monolithic PUT; SKU at top level with name/tier/capacity; all inline sub-config preserved; operationalState: Running) | [appgw_test.go](../internal/arm/appgw_test.go), [arm_test.go](../test/integration/arm_test.go) |
| DNS Zones | Full | N/A | `azurerm_dns_zone`, `azurerm_dns_a_record`, `azurerm_dns_aaaa_record`, `azurerm_dns_cname_record`, `azurerm_dns_txt_record`, `azurerm_dns_mx_record`, `azurerm_dns_srv_record`, `azurerm_dns_ns_record` | Full (auto-SOA + auto-NS on zone create; A, AAAA, CNAME, TXT, MX, SRV, NS, SOA record sets as children; cascade delete) | [dns_test.go](../internal/arm/dns_test.go), [arm_test.go](../test/integration/arm_test.go) |
| Storage Accounts | Full | Delegated to Azurite | `azurerm_storage_account`, `azurerm_storage_container` | Full (management plane; Azurite path-style endpoints; `listKeys` returns Azurite dev key; name uniqueness check; blob containers as child resources with cascade delete) | [storage_account_test.go](../internal/arm/storage_account_test.go), [storage_container_test.go](../internal/arm/storage_container_test.go), [arm_test.go](../test/integration/arm_test.go) |
| Key Vault | Full | None | `azurerm_key_vault` | Full (management plane; `vaultUri` computed; SKU/soft-delete defaults; `azurerm_key_vault_secret` data plane planned v0.2) | [keyvault_test.go](../internal/arm/keyvault_test.go), [arm_test.go](../test/integration/arm_test.go) |

## Identity

| Capability | Status | Notes |
|-----------|--------|-------|
| Service principal (client_id/secret) | Full | Accepts any credentials |
| Managed identity (IMDS) | None | Planned (v0.3) |
| Workload identity (OIDC federation) | None | Planned (v0.3) |
| Azure DevOps OIDC (`SYSTEM_OIDCREQUESTURI`) | None | Planned (v0.3) |
| ADO Service Connections CRUD | None | Planned (v0.3) |

## Developer Tooling

| Capability | Status | Proof | Notes |
|-----------|--------|-------|-------|
| Docker image | Full | [Dockerfile](../Dockerfile), [docker-compose.yml](../docker-compose.yml) | Multi-stage Go build, alpine runtime, `VOLUME /azemu`, healthcheck on `:4568` |
| Docker Compose | Full | [examples/terraform/README.md](../examples/terraform/README.md) | `docker compose up -d --build` is the default quick-start path |
| Wrapper script (`aztf`) | Full | [scripts/aztf](../scripts/aztf) | Starts azemu if absent, exports `SSL_CERT_FILE` + `ARM_*`, execs terraform |
| State export/import | Full | [file_test.go](../internal/store/file_test.go) | `GET /api/state/export`, `POST /api/state/import`, `POST /api/state/reset`; file-backed via `--persist` |
| `terraform test` example | Full | [main.tftest.hcl](../examples/terraform/main.tftest.hcl) | Native Terraform 1.6+ test; one `run "full_lifecycle"` block |
| Nix flake | Full | [flake.nix](../flake.nix) | `buildGoModule` for `cmd/azemu`; `devShells.default` with go + terraform |
| Binary releases | None | -- | Planned (goreleaser, Phase 5.9) |
| Fixtures/snapshots | None | -- | Planned (v0.2+) |
| Helm chart | None | -- | Planned (v0.3+) |

---

Status key:

- **Full**: implemented, tested, and the Proof column points at the test(s) that prove it.
- **Scaffold**: code exists but not validated or tested (no Proof link yet).
- **Stub**: endpoint returns 200 but no real logic.
- **None**: not implemented.
- **Planned**: on roadmap with target version; see `ROADMAP.md`.
