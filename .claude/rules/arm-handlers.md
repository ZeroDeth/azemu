---
paths:
  - "internal/arm/**/*.go"
---

# ARM handler rules

Loaded only when Claude is reading or editing files under `internal/arm/`.
Full ARM API contract reference is in `docs/CONVENTIONS.md`; this file lists
the rules a handler author MUST hold in working memory.

## Reference handlers

Use `internal/arm/vnet.go` and `internal/arm/subnet.go` as the canonical
pattern. They postdate the shared test helpers and the production middleware
stack. `internal/arm/resourcegroup.go` predates them and is a less complete
model.

## Required handler shape

Every handler MUST:

1. Extract path params with `chi.URLParam(r, "...")`.
2. Validate required fields. Return Azure error format on failure.
3. Use `writeJSON` and `writeAzureError` from `internal/arm/helpers.go` (these
   set `Content-Type: application/json` for you).
4. Log the operation with `zerolog` structured fields.
5. For child resources, return `404 ParentResourceNotFound` on PUT if the
   parent does not exist. See `subnet.go` for the canonical check.

## Required response shape

```json
{
  "id":       "/subscriptions/{sub}/resourceGroups/{rg}/...",
  "name":     "{name}",
  "type":     "Microsoft.{Namespace}/{resource}",
  "location": "uksouth",
  "tags":     {},
  "properties": { "provisioningState": "Succeeded" }
}
```

- LIST responses: `{"value": [...]}` wrapper.
- Error responses: `{"error": {"code": "...", "message": "..."}}`.
- `location` is lowercase, no spaces (`"uksouth"`, not `"UK South"`).

## Mandatory ARM behaviours

| Rule | Detail |
|------|--------|
| api-version required | Routes return 400 `MissingApiVersionParameter` if `?api-version=` is absent. The `RequireAPIVersion` middleware enforces this. |
| PUT idempotency | First write returns 201 Created; updates return 200 OK. |
| DELETE is async | 202 Accepted with a `Location` header pointing at an operation result URL. Subsequent GET returns 404. |
| HEAD for existence | 204 No Content if exists; 404 if not. No body. |
| Cascade delete | Deleting a parent (RG, vnet) MUST delete all children via store prefix match. |
| Azure headers | Every response includes `x-ms-request-id` and `x-ms-correlation-request-id`. The `AzureHeaders` middleware sets these. |

## Routing gotchas

- Register chi routes with **lowercase** path literals (`resourcegroups`,
  `virtualnetworks`, `subnets`). Real azurerm sends camelCase
  (`resourceGroups`); the `internal/middleware/pathcase.go` `NormalizePath`
  middleware lowercases canonical ARM literals before chi sees the request.
  Don't rely on exact-case matching.
- The handler test server (`newTestServer(t)` in `testutil_test.go`) wires
  the full middleware stack — write tests against it, not against the bare
  `arm.NewRouter(...)`.

## Adding a new resource type

Follow the checklist in `docs/CHECKLISTS.md`.
