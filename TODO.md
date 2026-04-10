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

These block `terraform apply` *before* any ARM HTTP request is made, so they
do not appear in `/api/unhandled`. They are gaps in the metadata response
shape that the azurerm v4.x provider parses at startup.

| # | Symptom | Root cause | Status | Notes |
|---|---|---|---|---|
| M1 | "does not support Azure Stack" rejection | `dataPlane` declared as `http://` but port 4566 is HTTPS, so `batch` and `sqlManagement` triggered the classifier | FIXED 2026-04-11 | service.go:35; regression test in service_test.go |
| M2 | "does not support Azure Stack" rejection (second classifier path) | `authentication.tenant` was the user's tenant UUID; `IsAzureStack` in go-azure-sdk requires the literal `"common"` regardless of which user tenant the env serves | FIXED 2026-04-11 | service.go:50; regression test in service_test.go |
| M3 | `unable to build authorizer for Storage API: ... endpoint "AzureStorage" is not supported in this Azure Environment` | metadata response is missing `resourceIdentifiers.azureStorage` (and possibly other per-service resource identifiers) | OPEN | Provider builds authorizers for every registered service at startup, fails fast on the first missing one. May need only a handful of `resourceIdentifiers.*` entries or may need many — unknown until M3 is fixed and the next failure surfaces. |

---

## Known Gaps

- Token endpoint does not return `ext_expires_in` or `expires_on` fields
- OIDC discovery does not include all fields that Azure Entra returns
- No async operation polling (DELETE returns 202 but operation URL is not implemented)
- No resource-level tags querying
- `api-version` parameter is accepted but not validated against known versions
- **chi route casing:** existing RG routes and the new VNet/Subnet routes use
  lowercase path literals (`resourcegroups`, `microsoft.network`,
  `virtualnetworks`) while Azure canonical paths are camelCase. chi v5 is
  case-sensitive with no normalisation middleware in the stack, so if real
  `azurerm` traffic sends camelCase segments none of these routes will match.
  Needs a path-lowercasing middleware (or route duplication) before Phase 1
  can claim `terraform apply` fidelity. Added 2026-04-10 during `feat/vnet-subnet`.
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

---

## Ideas (not committed)

- Postgres-backed store for multi-process CI
- gRPC health check endpoint
- Prometheus metrics endpoint
- ARM deployment template execution (massively complex, defer)
- Azure CLI (`az`) compatibility (beyond Terraform)
- Testcontainers-go integration module
