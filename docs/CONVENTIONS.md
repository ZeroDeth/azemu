# Conventions -- azemu

Detailed Go style, ARM API contracts, auth contracts, and testing strategy.

Path-scoped subsets of these rules are also mirrored in `.claude/rules/` so
Claude Code loads them conditionally when touching matching files. This file
is the full reference and the canonical source.

See also:

- `docs/ARCHITECTURE.md` — package layout and dependency direction
- `docs/CHECKLISTS.md` — add-a-resource, modify-store, before-commit flows
- `docs/PARITY.md` — what is actually implemented
- `.claude/rules/arm-handlers.md` — condensed handler rules (path-scoped)
- `.claude/rules/go-style.md` — condensed Go style (path-scoped)
- `.claude/rules/tests.md` — condensed test rules (path-scoped)

---

## 1. Go style

### Error handling

Wrap with context using `%w`. Bare returns lose caller context; swallowed
errors hide bugs.

```go
// CORRECT
if err != nil {
    return fmt.Errorf("put resource group %q: %w", name, err)
}

// WRONG: bare return
if err != nil {
    return err
}

// WRONG: swallowed (logs and continues)
if err != nil {
    log.Error().Err(err).Msg("failed")
    // continues execution
}
```

### Logging

Use structured zerolog fields, never printf-style.

```go
// CORRECT
log.Info().Str("resource_id", id).Str("method", r.Method).Msg("resource created")

// WRONG
log.Info().Msgf("created resource %s via %s", id, r.Method)
```

### HTTP handlers

All ARM endpoint handlers MUST:

1. Extract path parameters with `chi.URLParam(r, "name")`.
2. Validate required fields (return Azure error format on failure).
3. Use the shared `writeJSON` and `writeAzureError` helpers from
   `internal/arm/helpers.go` (they set `Content-Type: application/json` for
   you).
4. Log the operation with zerolog.
5. For child resources, return `404 ParentResourceNotFound` on PUT if the
   parent does not exist (see `internal/arm/subnet.go`).

```go
func (a *Router) getResourceGroup(w http.ResponseWriter, r *http.Request) {
    subID := chi.URLParam(r, "subscriptionID")
    name := chi.URLParam(r, "resourceGroupName")
    id := resourceGroupID(subID, name)

    res, ok := a.store.Get(id)
    if !ok {
        writeAzureError(w, http.StatusNotFound, "ResourceGroupNotFound",
            fmt.Sprintf("Resource group '%s' could not be found.", name))
        return
    }
    writeJSON(w, http.StatusOK, resourceGroupResponse(res))
}
```

---

## 2. ARM API fidelity rules

These define what "correct" means for azemu's ARM implementation. They are
the contract. Violating them is a bug.

### Mandatory behaviours

| Rule | Detail |
|------|--------|
| api-version required | All `/subscriptions/` routes MUST require `?api-version=` in query string. Return 400 `MissingApiVersionParameter` if absent. |
| PUT idempotency | Resource PUT is idempotent. Return 201 Created on first write, 200 OK on update. |
| DELETE is async | Resource DELETE returns 202 Accepted with `Location` header pointing to an operation result URL. Subsequent GET returns 404. |
| HEAD for existence | HEAD on a resource returns 204 No Content if exists, 404 Not Found if not. No response body. |
| Provider registration | Provider registration (`POST .../providers/{ns}/register`) always succeeds. Return `"registrationState": "Registered"`. |
| Location normalisation | `location` field in responses MUST be lowercase, no spaces (e.g., `"uksouth"` not `"UK South"`). |
| Cascade delete | Deleting a resource group MUST delete all resources within it via store prefix match. Same for VNet → child subnets. |
| Azure error format | All error responses MUST use `{"error": {"code": "...", "message": "..."}}`. |
| Azure headers | All responses MUST include `x-ms-request-id` and `x-ms-correlation-request-id` (UUIDs). The `AzureHeaders` middleware sets these. |
| Path case normalization | Register chi routes with lowercase literals (`resourcegroups`). The `NormalizePath` middleware lowercases canonical ARM literals before matching. |

### Required response shape

```json
{
  "id":       "/subscriptions/{sub}/resourceGroups/{name}",
  "name":     "{name}",
  "type":     "Microsoft.Resources/resourceGroups",
  "location": "uksouth",
  "tags":     {},
  "properties": {
    "provisioningState": "Succeeded"
  }
}
```

- LIST responses MUST be wrapped in `{"value": [...]}`.
- Error responses MUST use `{"error": {"code": "...", "message": "..."}}`.

### Intentionally NOT implemented (v0.1)

- Async operation polling (the `Location` header from DELETE is returned but
  the operation endpoint is not implemented).
- Resource locks.
- Tags on individual resources (supported in request, stored, but not queryable).
- ARM deployment templates.
- Activity logs.
- RBAC / role assignments.

---

## 3. Auth fidelity rules

### Token endpoint

- Accept any `client_id` / `client_secret` / `grant_type` without validation.
- Return a valid RS256-signed JWT with claims: `aud`, `iss`, `iat`, `nbf`,
  `exp`, `tid`, `oid`, `appid`, `sub`.
- `iss` format: `https://sts.windows.net/{tenantID}/`
- `aud`: `https://management.azure.com/`
- Token expiry: 1 hour from issue.
- Response fields: `access_token`, `token_type` ("Bearer"), `expires_in` (3600),
  `resource`.

### OIDC discovery

- `GET /{tenantID}/.well-known/openid-configuration`
- Must return `issuer`, `token_endpoint`, `jwks_uri`,
  `authorization_endpoint`, `response_types_supported`,
  `id_token_signing_alg_values_supported`.
- All URLs must use the request's `Host` header to construct absolute URLs.

### JWKS

- `GET /{tenantID}/discovery/v2.0/keys`
- Must return the RSA public key matching the JWT signing key.
- Key must include `kty`, `use`, `kid`, `n`, `e` fields.
- `kid` in JWKS must match `kid` in JWT header.

### TLS

- Self-signed ECDSA P-256 certificate with SANs `localhost` and `127.0.0.1`.
- Same cert is shared by both servers (`:4566` ARM and `:4567` metadata/auth).
- When `AZEMU_CERT_PATH` is set: cert+key are loaded from a PEM bundle at that
  path (mode `0600`) if it exists, or generated and persisted there. Trust
  once in the system keychain and the cert survives restarts.
- When `AZEMU_CERT_PATH` is unset: a fresh cert is generated on every startup
  and a cert-only export is written to OS temp.
- The Go-based azurerm provider checks the OS keychain, not `SSL_CERT_FILE`.
  On macOS use `security add-trusted-cert -r trustRoot -p ssl ...`.

---

## 4. Testing strategy

### Unit tests per package

| Package | Coverage target | What to test |
|---------|----------------|--------------|
| `internal/store` | 90% | Put/Get/Delete/List, cascade delete, Export/Import round-trip, concurrent access |
| `internal/arm` | 85% | Each resource handler: correct status codes, response shapes, error cases, api-version enforcement |
| `internal/auth` | 85% | JWT claims validation, OIDC doc fields, JWKS key match, token expiry |
| `internal/metadata` | 90% | Required fields present, URLs use correct host/port, IsAzureStack classifier negative cases |
| `internal/middleware` | 90% | api-version rejection, Azure headers present, metadata/auth exempt |
| `pkg/config` | 80% | Env var loading, defaults, fallthroughs |

### Test patterns

Use standard `testing` only. No testify, no gomock. Use `httptest` for HTTP
handler tests. Use `newTestServer(t)` from `internal/arm/testutil_test.go`
so tests run through the full middleware stack — bypassing middleware is how
the chi case-sensitivity bug shipped (see TODO.md M4).

```go
func TestResourceGroupCRUD(t *testing.T) {
    srv := newTestServer(t)
    base := srv.URL + "/subscriptions/sub1/resourcegroups/rg1"

    // Create
    resp := httpPut(t, withAPIVersion(base), `{"location":"uksouth"}`)
    assertStatus(t, resp, http.StatusCreated)

    // Read
    resp = httpGet(t, withAPIVersion(base))
    assertStatus(t, resp, http.StatusOK)

    // Delete
    resp = httpDelete(t, withAPIVersion(base))
    assertStatus(t, resp, http.StatusAccepted)

    // Verify gone
    resp = httpGet(t, withAPIVersion(base))
    assertStatus(t, resp, http.StatusNotFound)
}
```

- Table-driven tests when there are 3+ cases for the same function.
- Test names: `TestFunction_scenario` (e.g., `TestPut_cascadeDelete`).
- Test error paths, not just happy paths.
- Don't modify the code under test from inside a test file.

### Integration tests

Build tag: `//go:build integration`. Location: `test/integration/`. These
start an in-process `httptest` server with the full middleware stack and
make real HTTP requests — they test routing, middleware, handler, store,
and response wiring end-to-end without a real TCP listener.

### Terraform compatibility tests

Location: `test/terraform/`. Two levels:

1. `.tftest.hcl` files run with `terraform test` against a live azemu instance.
2. `test/compatibility/` — recorded HTTP traces from `terraform apply` against
   real Azure, diffed against azemu responses. (v0.2+, not yet implemented.)

---

## 5. Unhandled route strategy

Any request to a path that does not match a registered route MUST:

1. Log the full request (method, path, query, headers) at WARN level.
2. Return 501 Not Implemented with Azure error format:

   ```json
   {"error": {"code": "NotImplemented", "message": "azemu does not implement {method} {path}. See TODO.md."}}
   ```

3. Append the path to an in-memory "unhandled routes" list.
4. `GET /api/unhandled` dumps the list for debugging.

This is critical for Terraform compatibility work: when `terraform apply`
fails, the unhandled routes list tells you exactly what to implement next.

---

## 6. File and directory rules

### Do NOT modify without explicit approval

- `go.mod` / `go.sum` (dependency changes)
- `Dockerfile` (build pipeline)
- `LICENSE`
- `.github/` (CI workflows, when added)
- `.markdownlint.yaml` / `.pre-commit-config.yaml` (linter config — fix the
  code instead of weakening the rule)

### Do NOT create

- Vendor directory (`go mod vendor`).
- Generated protobuf files (not used).
- Binary files (they go in `bin/` which is gitignored).

### Naming conventions

| Item | Convention | Example |
|------|------------|---------|
| Go files | lowercase, underscore separated | `resource_group.go` |
| Test files | `*_test.go` alongside source | `resource_group_test.go` |
| ARM resource handlers | one file per resource type | `arm/vnet.go`, `arm/dns.go` |
| Test helpers | `testutil_test.go` or `helpers_test.go` | `arm/testutil_test.go` |
| Scripts | lowercase, no extension for bash | `scripts/aztf` |
| Root docs | UPPERCASE.md | `README.md`, `CHANGELOG.md`, `TASKS.md`, `TODO.md` |

---

## 7. Parity matrix discipline

`docs/PARITY.md` tracks what azemu implements. Update it whenever a resource
handler is added or changed.

Status values:

- **Full** — all CRUD operations implemented and tested
- **Stub** — endpoint exists, returns 200 but data is not persisted or validated
- **None** — not implemented
- **Planned** — on the roadmap, not yet started
