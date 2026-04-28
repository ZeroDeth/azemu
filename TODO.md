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

The bundle file is written with mode 0600 because it contains the private key.

---

## Known Gaps

- **Website mirror missing for ADR 0002 and ADR 0003.** PR #35 added
  `docs/adr/0002-azemu-plus-kind-for-aks-workload-deployments.md` and
  `docs/adr/0003-add-azure-cache-for-redis.md` but did not create the
  corresponding pages under `website/docs/resources/design-decisions/`
  or register them in `website/mkdocs.yml` nav. Required by
  `.claude/rules/docs-website.md`. Tracked as TASKS Phase 7.9 (ADR 0003
  mirror) and Phase 8.9 (ADR 0002 mirror); both should land alongside
  the implementation PR for the resource/scenario they describe so
  status, dates, and prose stay aligned. Precedent:
  `website/docs/resources/design-decisions/0001-delegate-storage-data-plane-to-azurite.md`.
- ADR 0002 implementation prose ("Optional Redis cache (per ADR 0003)")
  understates the dependency relative to TASKS Phase 8.7.1
  (`Depends on 8.2 and 7.7`). Reword to "Redis cache (per ADR 0003) is
  required by the multi-replica variant" when the ADR is updated next,
  or capture the variant split (ARM-only vs multi-replica) explicitly.
- ADR 0002 references the workload as both `expo-open-ota` (canonical
  GitHub repo name) and "Expo Updates server example". Pick one on the
  next edit; canonical is `expo-open-ota`.
- ADR 0002 open question on `:4566`/`:4567` reuse (line 183) cites port
  numbers without verifying against `docs/SETUP.md` (`:4566` ARM HTTP,
  `:4567` ARM HTTPS, `:4568` health, `:4569` ADO OIDC HTTP). Confirm
  port numbers in the ADR before Phase 8.7 build-out.
- Token endpoint does not return `ext_expires_in` or `expires_on` fields
- OIDC discovery does not include all fields that Azure Entra returns
- No async operation polling (DELETE returns 202 but operation URL is not implemented)
- No resource-level tags querying
- `api-version` parameter is accepted but not validated against known versions
- ~~**chi route casing:** existing RG routes and the new VNet/Subnet routes use
  lowercase path literals while Azure canonical paths are camelCase.~~
  **RESOLVED 2026-04-11** by `internal/middleware/pathcase.go` (M4 above).
- ~~**`store.Put` error ignored:** every handler (RG, VNet, Subnet) calls
  `_ = a.store.Put(id, res)` because `MemoryStore.Put` cannot fail today.~~
  **RESOLVED 2026-04-12** in the pre-Phase-4 hardening commit. All three
  handlers now check `store.Put` errors and return 500 with Azure error format.
- ~~**Tags returned as `null` on empty:** the shared response builders render
  `"tags": null` when the store has no tags, rather than `"tags": {}` as real
  Azure does.~~ **RESOLVED 2026-04-11** in Phase 2.5. `normaliseTags()` in
  `router.go` converts nil to `map[string]string{}`. Applied in
  `putResourceGroup` and `putVNet`. Pinned by
  `TestRG_PUT_NilTags_NormalisedToEmptyObject` and
  `TestVNet_PUT_NilTags_NormalisedToEmptyObject`.
- ~~**Inline subnets in VNet PUT body are dropped:**~~ **RESOLVED 2026-04-22**
  (task 6.8). Decision is to keep dropping inline subnets; this avoids a
  split-brain between the inline data and the child store entries that
  `getVNet` assembles at read time. Documented in `docs/PARITY.md` and
  pinned by `TestVNet_PUT_InlineSubnets_DroppedSilently`.
- ~~**`putResourceGroup` accepts empty location:** `resourcegroup.go` predates
  the validation pattern used in `vnet.go` and `subnet.go`.~~
  **RESOLVED 2026-04-11** during the Phase 2 closeout batch.
  `putResourceGroup` now returns 400 `InvalidRequestContent` for empty or
  whitespace-only location, matching vnet/subnet. Pinned by
  `TestRG_PUT_MissingLocation_Returns400` and
  `TestRG_PUT_WhitespaceOnlyLocation_Returns400`.
- ~~**OIDC and JWKS wiring leaks out of `internal/auth`:** `TokenService.Routes`
  mounts `/token`, but `OpenIDConfig` and `JWKS` are registered separately in
  `cmd/azemu/main.go`.~~ **RESOLVED 2026-04-11** in Phase 2.5.
  `TenantRoutes(chi.Router)` mounts oauth2 + OIDC + JWKS under one
  `/{tenantID}` group. `main.go` and `harness_test.go` each reduced to a
  single `r.Route("/{tenantID}", tokenSvc.TenantRoutes)` call.
- ~~**VNet and Subnet test coverage gaps:** `headSubnet` 77.8%, `deleteSubnet`
  81.8%, `writeVNetList` 85.7%.~~ **RESOLVED 2026-04-11** during the Phase 2
  closeout batch. All three functions now at 100%. See
  `TestSubnet_HEAD_NotFound_Returns404_EmptyBody`,
  `TestSubnet_DELETE_NotFound_Returns404`, and
  `TestVNet_LIST_ByRG_FiltersOutSubnets`. `internal/arm` package coverage
  climbed from 90.7% to 92.6%.
- ~~**`azureTimestamp` dead code:** declared in `internal/arm/router.go` but
  never called by any handler.~~ **RESOLVED 2026-04-11** by deletion during
  the Phase 2 closeout batch.

---

## Ideas (not committed)

- Postgres-backed store for multi-process CI
- ~~gRPC health check endpoint~~ **RESOLVED 2026-04-12** by plain-HTTP
  `GET /health` on `:4568` (Phase 3.12). gRPC adds a dependency; plain HTTP
  is simpler and sufficient for docker-compose and Kubernetes probes.
- Prometheus metrics endpoint
- ARM deployment template execution (massively complex, defer)
- Azure CLI (`az`) compatibility (beyond Terraform)
- Testcontainers-go integration module
