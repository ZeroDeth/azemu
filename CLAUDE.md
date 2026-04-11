# CLAUDE.md -- azemu

## 1) Project Identity

**azemu** is a Terraform-first, open-source local Azure emulator written in Go.
It intercepts `azurerm` Terraform provider calls by implementing Azure's metadata
service, mock OAuth2/OIDC auth, and a subset of the ARM REST API surface.

Domain: `azemu.dev`
Go module: `github.com/zerodeth/azemu` (vanity import `azemu.dev/azemu` planned)
Licence: MIT
Owner: Sherif Abdalla (ZeroDeth)

The project exists because:

- MiniBlue (miniblue.io) proved the `metadata_host` provider redirection works
  but has limited ARM coverage and no Azure DevOps identity story.
- LocalStack proved the developer workflow (wrapper CLI, state snapshots,
  Docker-first, `tflocal`) but its Azure support is commercial/alpha-only
  and the core repo is now archived.
- There is no open-source, Terraform-first Azure emulator with ADO OIDC flows.

---

## 2) Architecture Overview

```text
Developer / CI
    |
    v
Terraform CLI -----> HTTPS :4567 -----> Metadata Service (/metadata/endpoints)
    |                                    Auth Service (OAuth2, OIDC, JWKS)
    |
    +---------------> HTTP  :4566 -----> ARM Facade Router
                                           +-- Subscriptions / Tenants
                                           +-- Provider Registration
                                           +-- Resource Groups (CRUD)
                                           +-- [v0.2+: VNets, DNS, Storage...]
                                         |
                                         v
                                      State Store (in-memory)
                                         |
                                         v
                                      Export/Import (JSON file)
```

### How provider redirection works

The `hashicorp/azurerm` Terraform provider has a `metadata_host` field. When set,
the provider calls `https://{metadata_host}/metadata/endpoints` to discover all
Azure service URLs (ARM, auth, graph, etc.) instead of using built-in profiles.
The provider code does:

```go
environments.FromEndpoint(ctx, fmt.Sprintf("https://%s", metadataHost))
```

azemu serves this endpoint and returns URLs pointing back to itself, so all
subsequent ARM calls, token requests, and data plane calls stay local.

This requires:

- HTTPS on port 4567 with a self-signed cert (TLS mandatory for metadata)
- HTTP on port 4566 for ARM and data plane calls
- Mock OAuth2 token endpoint that returns valid JWTs
- ARM API surface with correct response shapes and error formats

### Package layout

```text
cmd/azemu/main.go              -- entrypoint, server setup, graceful shutdown
internal/
  metadata/service.go          -- /metadata/endpoints (the redirect root)
  auth/token.go                -- OAuth2 token endpoint, OIDC discovery, JWKS
  auth/tls.go                  -- self-signed cert generation, cert file export
  arm/router.go                -- ARM facade: subscriptions, providers, resources
  arm/resourcegroup.go         -- resource group CRUD (first resource type)
  arm/vnet.go                  -- [v0.2] virtual networks + subnets
  arm/dns.go                   -- [v0.2] DNS zones
  arm/storage.go               -- [v0.2] storage accounts (management plane)
  arm/helpers.go               -- shared ARM response builders, error formatting
  store/store.go               -- Store interface definition
  store/memory.go              -- in-memory implementation
  store/file.go                -- [v0.1] file-based persistence (JSON)
  middleware/azure.go          -- Azure headers, api-version enforcement
  middleware/logging.go        -- request/response logging with zerolog
  middleware/unhandled.go      -- catch-all for unrouted paths (log + 501)
pkg/
  config/config.go             -- env-based config with flag overrides
  armtypes/types.go            -- shared ARM request/response structs
scripts/
  aztf                         -- wrapper script (like tflocal for AWS)
  trust-cert.sh                -- cert trust helper for macOS/Linux
test/
  terraform/main.tf            -- basic azurerm provider config
  terraform/main.tftest.hcl    -- terraform test suite
  integration/smoke_test.go    -- HTTP-level integration tests
  compatibility/               -- recorded provider HTTP traces for diffing
docs/
  PARITY.md                    -- Full/Stub/None matrix per resource
  ARCHITECTURE.md              -- extended architecture docs
  CONTRIBUTING.md              -- contribution guide
```

### Dependency direction (enforced)

```text
cmd/azemu --> internal/* --> pkg/*
                         --> store.Store (interface)

internal packages MUST NOT import each other except:
  - arm/ may import store/ (to read/write resources)
  - arm/ may import pkg/armtypes/ (shared types)
  - middleware/ is standalone (no internal imports)
  - metadata/ imports only pkg/config/
  - auth/ is standalone
```

### Key dependencies (go.mod)

| Package | Purpose | Pinned |
|---------|---------|--------|
| `go-chi/chi/v5` | HTTP routing | ~v5.1 |
| `golang-jwt/jwt/v5` | JWT creation/validation | ~v5.2 |
| `google/uuid` | Request IDs | ~v1.6 |
| `rs/zerolog` | Structured logging | ~v1.33 |

No other dependencies. Standard library for TLS, crypto, testing, flags, JSON.
Do NOT add cobra, viper, urfave/cli, testify, or any other framework without
explicit approval.

---

## 3) Build & Test Commands

### Build

```bash
go build -o bin/azemu ./cmd/azemu
```

### Run

```bash
./bin/azemu
# HTTP on :4566, HTTPS on :4567
# Prints cert path to stdout on startup
```

### Run with options

```bash
./bin/azemu --port 4566 --tls-port 4567 --persist state.json
```

### Unit tests

```bash
go test ./... -v -count=1 -race
```

### Integration tests

```bash
go test ./test/integration/... -v -tags=integration
```

### Terraform smoke test

```bash
# Requires azemu running
export SSL_CERT_FILE=/tmp/azemu-cert.pem
export ARM_METADATA_HOSTNAME=localhost:4567
cd test/terraform
terraform init
terraform apply -auto-approve
terraform destroy -auto-approve
```

### Terraform test (HCL test framework)

```bash
# Requires azemu running
cd test/terraform
terraform test
```

### Docker

```bash
docker build -t azemu:latest .
docker run --rm -p 4566:4566 -p 4567:4567 azemu:latest
```

### Lint

```bash
go vet ./...
# golangci-lint if installed:
golangci-lint run ./...
```

### Full validation sequence (run all before any PR)

```bash
go vet ./...
go test ./... -v -count=1 -race
go build -o bin/azemu ./cmd/azemu
./bin/azemu &
sleep 2
go test ./test/integration/... -v -tags=integration
kill %1
```

---

## 4) Agent Operating Rules

### Planning and confirmation

- ALWAYS propose a PLAN before writing or modifying code. List:
  - files to create or modify
  - the approach in 2-3 sentences
  - which tests to add or update
- WAIT for confirmation before executing the plan.
- For trivial fixes (typos, formatting, import ordering), proceed without a plan.

### Edits

- For any file change, show a clear diff or description of what changed and why.
- Keep changes minimal, local, and reversible.
- NEVER change business logic without adding or updating tests.
- Prefer extending existing patterns over introducing new abstractions.

### Subagent usage

Claude Code can spawn subagents for independent, parallelisable work.
Use subagents when:

- Writing tests for multiple packages simultaneously
- Implementing independent ARM resource handlers
- Running lint + build + test in parallel

Do NOT use subagents when:

- Changes are sequential and interdependent
- Modifying shared interfaces (Store, ARM helpers, middleware)
- Debugging a test failure (need full context)

Subagent task template:

```text
## Subagent Task: [short name]
### Scope
[exactly which files to create/modify]
### Context
[what the subagent needs to know]
### Constraints
[files NOT to touch, patterns to follow]
### Verification
[how to confirm the work is correct]
```

### Safety

- Do NOT delete files without explicit approval.
- Do NOT modify go.mod to add new dependencies without approval.
- Do NOT run `git push`, `git push --force`, or `git reset --hard`.
- Do NOT commit to `main` directly. All work on feature branches.
- Do NOT generate or commit secrets, tokens, or private keys to the repo.
  The self-signed cert is generated at runtime, never stored in source.

### Code review mode

When asked to review code:

1. Check for correctness against ARM API contracts
2. Check error handling (no swallowed errors, proper wrapping)
3. Check test coverage (new code must have tests)
4. Check for import cycles between internal packages
5. Check response shapes match Azure API format
6. Flag any TODO/FIXME items
7. Verify no new dependencies added without justification

---

## 5) Go Conventions (azemu-specific)

### Error handling

```go
// CORRECT: wrap with context
if err != nil {
    return fmt.Errorf("put resource group %q: %w", name, err)
}

// WRONG: bare return
if err != nil {
    return err
}

// WRONG: swallowed
if err != nil {
    log.Error().Err(err).Msg("failed")
    // continues execution
}
```

### Logging

```go
// CORRECT: structured fields
log.Info().Str("resource_id", id).Str("method", r.Method).Msg("resource created")

// WRONG: printf-style
log.Info().Msgf("created resource %s via %s", id, r.Method)
```

### HTTP handlers

All ARM endpoint handlers MUST:

1. Extract path parameters with `chi.URLParam(r, "name")`
2. Validate required fields (return Azure error format on failure)
3. Set `Content-Type: application/json` on all responses
4. Use the shared `writeJSON` and `writeAzureError` helpers
5. Log the operation with zerolog

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

### ARM response shapes

Every ARM resource response MUST include:

```json
{
    "id": "/subscriptions/{sub}/resourceGroups/{name}",
    "name": "{name}",
    "type": "Microsoft.Resources/resourceGroups",
    "location": "uksouth",
    "tags": {},
    "properties": {
        "provisioningState": "Succeeded"
    }
}
```

List responses MUST be wrapped in `{"value": [...]}`.
Error responses MUST use `{"error": {"code": "...", "message": "..."}}`.

### Test patterns

```go
func TestResourceGroupCRUD(t *testing.T) {
    store := store.NewMemoryStore()
    router := arm.NewRouter(store)

    // Use httptest for handler tests
    srv := httptest.NewServer(router.Handler())
    defer srv.Close()

    // Create
    resp := httpPut(t, srv.URL+"/subscriptions/sub1/resourcegroups/rg1?api-version=2023-07-01",
        `{"location":"uksouth"}`)
    assertStatus(t, resp, http.StatusCreated)
    assertJSONField(t, resp, "name", "rg1")

    // Read
    resp = httpGet(t, srv.URL+"/subscriptions/sub1/resourcegroups/rg1?api-version=2023-07-01")
    assertStatus(t, resp, http.StatusOK)

    // Delete
    resp = httpDelete(t, srv.URL+"/subscriptions/sub1/resourcegroups/rg1?api-version=2023-07-01")
    assertStatus(t, resp, http.StatusAccepted)

    // Verify gone
    resp = httpGet(t, srv.URL+"/subscriptions/sub1/resourcegroups/rg1?api-version=2023-07-01")
    assertStatus(t, resp, http.StatusNotFound)
}
```

---

## 6) ARM API Fidelity Rules

These rules define what "correct" means for azemu's ARM implementation.
They are the contract. Violating them is a bug.

### Mandatory behaviours

| Rule | Detail |
|------|--------|
| api-version required | All `/subscriptions/` routes MUST require `?api-version=` in query string. Return 400 `MissingApiVersionParameter` if absent. |
| PUT idempotency | Resource PUT is idempotent. Return 201 Created on first write, 200 OK on update. |
| DELETE is async | Resource DELETE returns 202 Accepted with `Location` header pointing to an operation result URL. Subsequent GET returns 404. |
| HEAD for existence | HEAD on a resource returns 204 No Content if exists, 404 Not Found if not. No response body. |
| Provider registration | Provider registration (`POST .../providers/{ns}/register`) always succeeds. Return `"registrationState": "Registered"`. |
| Location normalisation | `location` field in responses MUST be lowercase, no spaces (e.g., `"uksouth"` not `"UK South"`). |
| Cascade delete | Deleting a resource group MUST delete all resources within it (store prefix match). |
| Azure error format | All error responses MUST use `{"error": {"code": "...", "message": "..."}}`. |
| Azure headers | All responses MUST include `x-ms-request-id` and `x-ms-correlation-request-id` (UUIDs). |

### Intentionally NOT implemented (v0.1)

- Async operation polling (the `Location` header from DELETE is returned but the operation endpoint is not implemented)
- Resource locks
- Tags on individual resources (supported in request, stored, but not queryable)
- ARM deployment templates
- Activity logs
- RBAC / role assignments

---

## 7) Auth Fidelity Rules

### Token endpoint

- Accept any `client_id` / `client_secret` / `grant_type` without validation.
- Return a valid RS256-signed JWT with claims: `aud`, `iss`, `iat`, `nbf`, `exp`, `tid`, `oid`, `appid`, `sub`.
- `iss` format: `https://sts.windows.net/{tenantID}/`
- `aud`: `https://management.azure.com/`
- Token expiry: 1 hour from issue.
- Response fields: `access_token`, `token_type` ("Bearer"), `expires_in` (3600), `resource`.

### OIDC discovery

- `GET /{tenantID}/.well-known/openid-configuration`
- Must return `issuer`, `token_endpoint`, `jwks_uri`, `authorization_endpoint`,
  `response_types_supported`, `id_token_signing_alg_values_supported`.
- All URLs must use the request's `Host` header to construct absolute URLs.

### JWKS

- `GET /{tenantID}/discovery/v2.0/keys`
- Must return the RSA public key matching the JWT signing key.
- Key must include `kty`, `use`, `kid`, `n`, `e` fields.
- `kid` in JWKS must match `kid` in JWT header.

### TLS

- Self-signed ECDSA P-256 certificate generated at startup.
- SANs: `localhost`, `127.0.0.1`.
- Cert written to temp file, path logged to stdout.
- User sets `SSL_CERT_FILE` to trust it.

---

## 8) Testing Strategy

### Unit tests (per package)

| Package | What to test | Min coverage |
|---------|-------------|--------------|
| `internal/store` | Put/Get/Delete/List, cascade delete, Export/Import round-trip, concurrent access | 90% |
| `internal/arm` | Each resource handler: correct status codes, response shapes, error cases, api-version enforcement | 85% |
| `internal/auth` | JWT claims validation, OIDC doc fields, JWKS key match, token expiry | 85% |
| `internal/metadata` | Endpoint response contains all required fields, URLs use correct host/port | 90% |
| `internal/middleware` | api-version rejection, Azure headers present, metadata/auth exempt from api-version | 90% |
| `pkg/config` | Env var loading, defaults, flag overrides | 80% |

### Integration tests (test layout)

Location: `test/integration/`
Build tag: `//go:build integration`

These start a real azemu server on random ports and make HTTP requests.
They test the full request flow: routing, middleware, handler, store, response.

### Terraform compatibility tests

Location: `test/terraform/`

Two levels:

1. `.tftest.hcl` files: run with `terraform test` against a live azemu instance.
2. `test/compatibility/`: recorded HTTP traces from `terraform apply` against
   real Azure, diffed against azemu responses. (v0.2+)

### What to test when adding a new ARM resource

Checklist:

- [ ] Unit test: PUT returns 201 on create, 200 on update
- [ ] Unit test: GET returns 200 with correct shape, 404 when missing
- [ ] Unit test: DELETE returns 202, subsequent GET returns 404
- [ ] Unit test: HEAD returns 204/404
- [ ] Unit test: LIST returns `{"value": [...]}` wrapper
- [ ] Unit test: Missing api-version returns 400
- [ ] Unit test: Invalid body returns 400 with Azure error format
- [ ] Integration test: full CRUD cycle via HTTP
- [ ] Terraform test: resource can be created and destroyed
- [ ] PARITY.md updated with Full/Stub/None status

---

## 9) Unhandled Route Strategy

Any request to a path that does not match a registered route MUST:

1. Log the full request (method, path, query, headers) at WARN level
2. Return 501 Not Implemented with Azure error format:

   ```json
   {"error": {"code": "NotImplemented", "message": "azemu does not implement {method} {path}. See TODO.md."}}
   ```

3. Append the path to an in-memory "unhandled routes" list
4. Expose `GET /api/unhandled` on the HTTP port to dump the list (for debugging)

This is critical for Terraform compatibility work. When `terraform apply` fails,
the unhandled routes list tells you exactly what to implement next.

---

## 10) File and Directory Rules

### Do NOT modify without explicit approval

- `go.mod` / `go.sum` (dependency changes)
- `Dockerfile` (build pipeline)
- `LICENSE`
- `.github/` (CI workflows, when added)

### Do NOT create

- Vendor directory (`go mod vendor`)
- Generated protobuf files (not used)
- Binary files (they go in `bin/` which is gitignored)

### Naming conventions

| Item | Convention | Example |
|------|-----------|---------|
| Go files | lowercase, underscore separated | `resource_group.go` |
| Test files | `*_test.go` alongside source | `resource_group_test.go` |
| ARM resource handlers | one file per resource type | `arm/vnet.go`, `arm/dns.go` |
| Test helpers | `testutil_test.go` or `helpers_test.go` | `arm/testutil_test.go` |
| Scripts | lowercase, no extension for bash | `scripts/aztf` |
| Docs | UPPERCASE.md for root docs | `PARITY.md`, `TODO.md` |

---

## 11) Parity Matrix Discipline

The file `docs/PARITY.md` tracks what azemu implements. Update it whenever
a resource handler is added or changed.

Format:

```markdown
| Resource | ARM CRUD | Data Plane | Terraform Resource | Status |
|----------|----------|------------|-------------------|--------|
| Resource Groups | Full | N/A | azurerm_resource_group | Full |
| VNets | None | N/A | azurerm_virtual_network | Planned |
| Subnets | None | N/A | azurerm_subnet | Planned |
```

Status values:

- **Full**: all CRUD operations implemented and tested
- **Stub**: endpoint exists, returns 200 but data is not persisted or validated
- **None**: not implemented
- **Planned**: on the roadmap, not yet started

---

## 12) Checklists

### Checklist: Add a new ARM resource type

- [ ] Create `internal/arm/{resource}.go` with CRUD handlers
- [ ] Register routes in `internal/arm/router.go`
- [ ] Add shared types to `pkg/armtypes/` if needed
- [ ] Write unit tests: `internal/arm/{resource}_test.go`
- [ ] Write integration test case in `test/integration/`
- [ ] Add Terraform example in `test/terraform/`
- [ ] Add `.tftest.hcl` test case
- [ ] Update `docs/PARITY.md`
- [ ] Update `README.md` resource table
- [ ] Run full validation sequence

### Checklist: Modify store interface

- [ ] Update `internal/store/store.go` interface
- [ ] Update `internal/store/memory.go` implementation
- [ ] Update `internal/store/file.go` if it exists
- [ ] Update all store tests
- [ ] Verify no arm/ tests break
- [ ] Run full validation sequence

### Checklist: Before any commit

- [ ] `go vet ./...` passes
- [ ] `go test ./... -count=1 -race` passes
- [ ] `go build -o bin/azemu ./cmd/azemu` succeeds
- [ ] No new dependencies added without approval
- [ ] No TODO without a tracking issue reference
