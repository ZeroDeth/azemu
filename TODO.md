# TODO.md -- azemu

Unimplemented endpoints and future work. Populated during Terraform compatibility
testing (Phase 1). Each entry records when the endpoint was first seen, what
called it, and whether it blocks `terraform apply`.

---

## Unhandled Endpoints (discovered during terraform apply)

| Endpoint | Method | Caller | Blocks apply? | Added |
|----------|--------|--------|--------------|-------|
| (none yet) | | | | |

---

## Known Gaps

- Token endpoint does not return `ext_expires_in` or `expires_on` fields
- OIDC discovery does not include all fields that Azure Entra returns
- No async operation polling (DELETE returns 202 but operation URL is not implemented)
- No resource-level tags querying
- `api-version` parameter is accepted but not validated against known versions

---

## Ideas (not committed)

- Postgres-backed store for multi-process CI
- gRPC health check endpoint
- Prometheus metrics endpoint
- ARM deployment template execution (massively complex, defer)
- Azure CLI (`az`) compatibility (beyond Terraform)
- Testcontainers-go integration module
