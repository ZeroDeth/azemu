---
paths:
  - "**/*_test.go"
---

# Test rules

Loaded only when Claude is reading or editing test files.

## Setup

For ARM handler tests, use `newTestServer(t)` from
`internal/arm/testutil_test.go`. It wires the production middleware stack
(`NormalizePath` → `AzureHeaders` → `RequireAPIVersion`) so tests exercise
the same code path as real requests.

Don't bypass middleware by calling `arm.NewRouter(...)` directly — that's how
the chi case-sensitivity bug shipped (M4 in `TODO.md`). The smoke test passed
in lowercase while real azurerm camelCase requests landed in `/api/unhandled`.

## Helpers

Use the shared helpers from `testutil_test.go`. Don't roll your own:

- `httpGet`, `httpGetRaw`, `httpPut`, `httpHead`, `httpDelete`
- `assertStatus`, `decodeJSON`, `readBody`
- `withAPIVersion(url)` auto-injects `?api-version=2023-09-01`

## Style

- Standard `testing` package only. No testify, no gomock.
- Use table-driven tests when there are 3+ cases for the same function.
- Test names: `TestFunction_scenario` (e.g., `TestPut_cascadeDelete`).
- Test error paths, not just happy paths.
- Use `httptest` for HTTP handler tests.
- Don't modify the code under test from inside a test file.

## Coverage targets per package

| Package | Min coverage | What to cover |
|---------|--------------|---------------|
| `internal/store` | 90% | Put/Get/Delete/List, cascade delete, Export/Import round-trip, concurrent access |
| `internal/arm` | 85% | Status codes, response shapes, error cases, api-version enforcement per resource |
| `internal/auth` | 85% | JWT claims, OIDC doc fields, JWKS key match, token expiry |
| `internal/metadata` | 90% | Required fields present, URLs use correct host/port, IsAzureStack classifier negative cases |
| `internal/middleware` | 90% | api-version rejection, Azure headers present, metadata/auth exempt |
| `pkg/config` | 80% | Env var loading, defaults, fallthroughs |

## Integration tests

Build tag: `//go:build integration`. Location: `test/integration/`. Use
`httptest.NewServer` with the full middleware stack — these are in-process,
not real TCP listeners.

## Adding tests for a new ARM resource

Follow the test-cases section of the `add-resource` skill
(`.claude/skills/add-resource/SKILL.md`).
