# ARM Fidelity

## What "emulate" means

azemu does not just return 200 OK. It implements the ARM contract faithfully
enough that the `hashicorp/azurerm` provider accepts responses without
modification. Every resource in the [Parity Matrix](parity-matrix.md) passes
a real `terraform apply` + `terraform destroy` round-trip.

## The contract

azemu implements these ARM behaviors:

| Behavior | What azemu does |
|----------|----------------|
| api-version required | Every ARM request must include `?api-version=`. Returns 400 `MissingApiVersionParameter` if absent. |
| PUT idempotency | 201 Created on first write, 200 OK on update. Same resource, same result. |
| DELETE is async | Returns 202 Accepted with a `Location` header. The resource is removed immediately from state. |
| HEAD for existence | Returns 204 No Content if the resource exists, 404 Not Found if not. No response body. |
| Provider registration | `POST .../providers/{ns}/register` always succeeds. Returns `"registrationState": "Registered"`. |
| Location normalization | The `location` field is always lowercase with no spaces: `"uksouth"`, not `"UK South"`. |
| Cascade delete | Deleting a resource group removes all resources within it. Same for parent-child relationships (VNet removes child subnets). |
| Azure error format | All errors use `{"error": {"code": "...", "message": "..."}}`. |
| Azure headers | Every response includes `x-ms-request-id` and `x-ms-correlation-request-id` (UUIDs). |

## Why this matters

Other emulators and mocks return simplified responses that work in unit tests
but fail against the real azurerm provider. The provider validates response
shapes, checks specific headers, and follows the async polling pattern for
deletions. azemu's bar is not "does the test pass" but "does `terraform apply`
succeed against unmodified azurerm v4.x."

Five classifier-class bugs were found and fixed during azemu's first
end-to-end Terraform run. Each one represents a contract detail that a simpler
mock would silently get wrong:

- HTTP vs HTTPS classification: the provider rejects `http://` resource manager
  URLs as Azure Stack.
- Missing `x-ms-request-id` headers: the provider validation fails without them.
- Wrong `location` casing: the provider comparison fails if location is not
  lowercase.
- Missing `tags` field: the provider expects `"tags": {}`, not null.
- Case-sensitive chi routes vs camelCase azurerm paths: the provider sends
  `resourceGroups`, routes must accept both cases.

## Not yet implemented

These ARM behaviors are intentionally deferred:

- Async operation polling: the `Location` header from DELETE is returned, but
  the polling endpoint is not yet implemented.
- Resource locks.
- Tags queries: tags are stored but not queryable via the tags API.
- ARM deployment templates.
- Activity logs.
- RBAC and role assignments.
