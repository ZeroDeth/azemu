---
name: add-resource
description: Add a new ARM resource type to azemu end-to-end (CRUD + HEAD handlers, chi route registration, unit tests, integration test, PARITY entry, README support table). Use when implementing a Microsoft.* resource such as a DNS zone, storage account, or key vault.
---

# Add a new ARM resource type

Follow these steps in order. Do not skip steps. The `vnet` and `subnet`
resources are the canonical references; use them as the template for
both production code and tests.

## 1. Create the handler file

Create `internal/arm/{resource}.go` with CRUD + HEAD handlers. Follow
the pattern in `internal/arm/vnet.go` and `internal/arm/subnet.go`.
`internal/arm/resourcegroup.go` predates the shared helpers and is a
weaker model; do not copy from it.

Each handler must:

- Extract path params with `chi.URLParam(r, "...")`.
- Validate required fields; return Azure error format on failure via
  `writeAzureError` from `internal/arm/helpers.go`.
- Return the canonical ARM response shape via `writeJSON`:
  `id`, `name`, `type`, `location` (lowercase no spaces), `tags`,
  `properties` including `provisioningState: "Succeeded"`.
- Log with structured zerolog fields (no `Msgf`).
- For child resources, return `404 ParentResourceNotFound` on PUT when
  the parent resource does not exist; see `subnet.go` for the canonical
  check.

## 2. Register routes

Register routes in `internal/arm/router.go` using **lowercase** chi
path literals (`virtualnetworks`, not `virtualNetworks`). The
`internal/middleware/pathcase.go` `NormalizePath` middleware lowercases
incoming canonical ARM literals before chi sees them.

## 3. Add shared types if needed

Add shared types to `pkg/armtypes/` only if the resource shape is
reused across packages. Resource-private types stay in the handler file.

## 4. Write unit tests

Create `internal/arm/{resource}_test.go` using `newTestServer(t)` from
`internal/arm/testutil_test.go`. Cover every case below:

- [ ] PUT returns 201 on create, 200 on update
- [ ] GET returns 200 with correct shape, 404 when missing
- [ ] DELETE returns 202; subsequent GET returns 404
- [ ] HEAD returns 204 when exists, 404 when missing; no body either way
- [ ] LIST returns `{"value": [...]}` wrapper
- [ ] Missing `api-version` parameter returns 400 `MissingApiVersionParameter`
- [ ] Invalid body returns 400 with Azure error format
- [ ] For child resources: PUT returns 404 `ParentResourceNotFound`
      when the parent does not exist
- [ ] Cascade delete: deleting the parent removes the child (if this
      resource has children)
- [ ] Response includes `x-ms-request-id` and `x-ms-correlation-request-id`
      headers (the `AzureHeaders` middleware sets these; just verify
      they appear)

Use `decodeJSON` and map assertions from the test helpers. Use
table-driven tests when there are 3+ cases for the same function. Test
names follow `TestFunction_scenario`.

## 5. Add an integration test case

Add a case to `test/integration/arm_test.go` that exercises the full
lifecycle (create, read, update, delete) through the production
middleware stack.

## 6. Add a Terraform example

Add the resource to `test/terraform/main.tf` so the end-to-end loop
(`terraform apply && terraform destroy`) covers it. If the resource
needs a separate .tf file, add it under `test/terraform/`.

## 7. Update docs

- `docs/PARITY.md`: add a row for the new resource with Full / Stub /
  None status.
- `README.md`: if the resource is user-visible, add it to the "Current
  support" table.

## 8. Run the full validation sequence

```bash
go vet ./...
go test ./... -v -count=1 -race
go build -o bin/azemu ./cmd/azemu
```

For changes that affect the ARM surface, also run the end-to-end
Terraform loop via the `validate-terraform` skill.

## 9. Before committing

Run the `before-commit` skill (`/before-commit`).

## When to delegate to a subagent

If the resource is a larger piece of work (new namespace, multiple
subresources, unfamiliar API contract), delegate the whole thing to the
`arm-resource-implementer` subagent instead of running these steps
sequentially. The subagent runs in isolated context, which is useful
when the implementation spans many files.
