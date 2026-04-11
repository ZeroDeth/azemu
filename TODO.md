# TODO.md -- azemu

Unimplemented endpoints and future work. Populated during Terraform compatibility
testing (Phase 1). Each entry records when the endpoint was first seen, what
called it, and whether it blocks `terraform apply`.

---

## Unhandled Endpoints (discovered during terraform apply)

| Endpoint | Method | Caller | Blocks apply? | Added |
|----------|--------|--------|--------------|-------|
| (none yet — provider has not made it past initialization to ARM call phase) | | | | |

## Provider initialization gaps (discovered during fix/metadata-classifier-bugs)

Five blockers were uncovered and fixed during the first end-to-end
`terraform apply` attempt against azemu. All five are documented here as a
post-mortem so the next contributor knows the recipe.

| # | Symptom | Root cause | Status | Fix |
|---|---|---|---|---|
| M1 | "does not support Azure Stack" rejection (first classifier path) | `dataPlane` declared as `http://` but port 4566 is HTTPS, so `batch` and `sqlManagement` triggered the classifier | FIXED 2026-04-11 | `internal/metadata/service.go` — switched dataPlane to `https://`; pinned by `TestMetadata_DataPlaneFieldsAreHTTPS` and `TestMetadata_AllLocalhostURLsUseHTTPS` |
| M2 | "does not support Azure Stack" rejection (second classifier path) | `authentication.tenant` was the user's tenant UUID; `IsAzureStack` in go-azure-sdk requires the literal `"common"` regardless of which user tenant the env serves | FIXED 2026-04-11 | `internal/metadata/service.go` — set `tenant: "common"`; pinned by `TestMetadata_NotClassifiedAsAzureStack` |
| M3 | `unable to build authorizer for Storage API: ... endpoint "AzureStorage" is not supported in this Azure Environment` | The entire metadata response had hand-rolled field names that did not match real Azure: `portalEndpoint` vs `portal`, `graphEndpoint` vs `graph`, `appInsights` vs `appInsightsResourceId`, `suffixes.storageEndpoint` vs `suffixes.storage`, etc. go-azure-sdk silently saw missing fields and failed when constructing per-service authorizers. | FIXED 2026-04-11 | Full canonical-schema rewrite of `internal/metadata/service.go` against ground truth from `https://management.azure.com/metadata/endpoints?api-version=2022-09-01`. Pinned by `TestMetadata_CanonicalFieldNames` and `TestMetadata_CanonicalSuffixNames`. |
| M4 | After M1-M3 fixed, the apply called the ARM router but every call landed in `/api/unhandled` with paths like `/subscriptions/.../resourceGroups/azemu-test-rg`. | chi v5 is case-sensitive and azemu's routes are registered in lowercase (`resourcegroups`). Real azurerm sends Azure-canonical camelCase (`resourceGroups`). No request from real Terraform could ever match a chi route. The `make smoke` curl test was a false positive because it was hand-constructed in lowercase. ALSO: the provider concatenates `metadata_host` with `/metadata/endpoints` and emits a leading `//` that chi treats as a separate route. | FIXED 2026-04-11 | New `internal/middleware/pathcase.go` with `NormalizePath` middleware that lowercases known ARM literal segments (case-insensitive) AND collapses runs of `/`. Wired into the production middleware stack BEFORE `RequireAPIVersion`. Pinned by 8 tests in `pathcase_test.go`. |
| M5 | `terraform destroy` failed at the polling stage with `internal-error: a polling status of Failed should be surfaced as a PollingFailedError` | Provider calls `GET /subscriptions/.../resourceGroups/{rg}/resources?$expand=...&$top=10` to enumerate child resources before issuing the RG DELETE. azemu didn't implement this endpoint and 501'd. The provider's polling logic interpreted the 501 as "operation Failed" and emitted a misleading internal-error message. | FIXED 2026-04-11 | New handler `listResourceGroupResources` in `internal/arm/router.go` that returns `{"value": []}` for an empty RG and a populated array otherwise. OData query params accepted and ignored. Pinned by 4 tests in `rg_resources_test.go`. |

## TLS trust friction (resolved structurally)

Phase 1.3 debugging required four manual `security add-trusted-cert` GUI
prompts (one per azemu restart) because azemu generated a fresh self-signed
certificate at every startup. This is fixed structurally by the
`AZEMU_CERT_PATH` environment variable: when set, azemu loads the cert+key
from a stable PEM bundle file if one exists, or generates and persists a
fresh pair otherwise. Trust the cert in the system keychain once, restart
the binary as many times as you like.

Recommended usage in flox or any iteration loop:

```bash
mkdir -p .azemu
AZEMU_CERT_PATH=$PWD/.azemu/cert-bundle.pem ./bin/azemu
```

The bundle file is written with mode 0600 because it includes a private key.

---

## Known Gaps

- Token endpoint does not return `ext_expires_in` or `expires_on` fields
- OIDC discovery does not include all fields that Azure Entra returns
- No async operation polling (DELETE returns 202 but operation URL is not implemented)
- No resource-level tags querying
- `api-version` parameter is accepted but not validated against known versions
- ~~**chi route casing:** existing RG routes and the new VNet/Subnet routes use
  lowercase path literals while Azure canonical paths are camelCase.~~
  **RESOLVED 2026-04-11** by `internal/middleware/pathcase.go` (M4 above).
- **`store.Put` error ignored:** every handler (RG, VNet, Subnet) calls
  `_ = a.store.Put(id, res)` because `MemoryStore.Put` cannot fail today. When
  the file-backed store lands (Phase 4), these sites will silently lose writes.
  Pattern needs a codebase-wide fix before Phase 4 merges.
- **Tags returned as `null` on empty:** the shared response builders render
  `"tags": null` when the store has no tags, rather than `"tags": {}` as real
  Azure does. Matches existing RG behaviour; may need normalisation if a
  Terraform consumer rejects null.
- **Inline subnets in VNet PUT body are dropped:** azemu v0.1 only recognises
  subnets created via the separate `.../subnets/{name}` endpoint, matching
  how `azurerm_subnet` issues writes. Real ARM accepts both. Documented in
  `docs/PARITY.md`.
- **`putResourceGroup` accepts empty location:** `resourcegroup.go` predates
  the validation pattern used in `vnet.go` and `subnet.go` (which return 400
  `InvalidRequestContent` when `location` is missing). A PUT with body `{}`
  is currently accepted and stored with `location: ""`. Pinned by
  `TestRG_PUT_MissingLocation_CurrentlyAccepted` in `internal/arm/rg_test.go`;
  flip the assertion to `StatusBadRequest` when the handler is brought in
  line with the newer resources.
- **OIDC and JWKS wiring leaks out of `internal/auth`:** `TokenService.Routes`
  mounts `/token`, but `OpenIDConfig` and `JWKS` are registered separately in
  `cmd/azemu/main.go` at `/{tenantID}/.well-known/openid-configuration` and
  `/{tenantID}/discovery/v2.0/keys`. Move both registrations into
  `Routes`/`RoutesV2` so the package owns its full public surface. Surfaced
  while writing `internal/auth/token_test.go`; the test helper had to
  replicate the production wiring verbatim.
- **VNet and Subnet test coverage gaps:** `headSubnet` 77.8%, `deleteSubnet`
  81.8%, `writeVNetList` 85.7%. These gaps predate the Phase 2 unit-test
  slice; Slice B (RG CRUD tests) was scoped not to touch the VNet/Subnet
  test files. Backfill in a dedicated VNet/Subnet test-extension PR before
  Phase 2 closeout.
- **`azureTimestamp` dead code:** declared in `internal/arm/router.go` but
  never called by any handler. Flagged at 0% coverage during the Phase 2
  slice. Delete in a small cleanup commit rather than writing a test for
  unreachable code.

---

## Ideas (not committed)

- Postgres-backed store for multi-process CI
- gRPC health check endpoint
- Prometheus metrics endpoint
- ARM deployment template execution (massively complex, defer)
- Azure CLI (`az`) compatibility (beyond Terraform)
- Testcontainers-go integration module
