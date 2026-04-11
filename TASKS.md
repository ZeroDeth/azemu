# TASKS.md -- azemu Implementation Plan

Version: 0.1
Last updated: 2026-04-11
Status: Phase 1 acceptance MET (terraform apply+destroy round-trip green). Current focus: Phase 2 (test coverage backfill).

> **Out-of-phase work on `feat/vnet-subnet` (2026-04-10):** VNets + Subnets
> (Phase 6) were implemented ahead of Phase 1 acceptance because the feature
> was requested before TASKS.md was consulted. The work is self-contained,
> shipped 25 unit tests + 1 integration test, and was retroactively validated
> by the Phase 1 end-to-end run on `fix/metadata-classifier-bugs`.

---

## Phase 0: Bootstrap (make it compile)

Goal: scaffold compiles, binary starts, responds to basic curl requests.

| # | Task | File(s) | Status | Notes |
|---|------|---------|--------|-------|
| 0.1 | Run `go mod tidy`, resolve deps, generate `go.sum` | `go.mod`, `go.sum` | DONE | |
| 0.2 | Fix compilation errors in scaffold | all `*.go` | DONE | Removed unused `crypto/x509` and `encoding/pem` imports |
| 0.3 | Verify `go build ./cmd/azemu` produces binary | `cmd/azemu/main.go` | DONE | |
| 0.4 | Verify binary starts, listens on :4566 and :4567 | | DONE | |
| 0.5 | Verify metadata endpoint returns JSON | `internal/metadata/service.go` | DONE | `curl -sk https://localhost:4567/metadata/endpoints` |
| 0.6 | Verify token endpoint returns JWT | `internal/auth/token.go` | DONE | `curl -sk -X POST https://localhost:4567/{tenant}/oauth2/v2.0/token` |
| 0.7 | Verify resource group CRUD via curl | `internal/arm/router.go` | DONE | PUT/GET/DELETE/HEAD/LIST |
| 0.8 | Verify api-version enforcement | `internal/middleware/azure.go` | DONE | Bare request returns 400 |

Acceptance: `make smoke` passes (start server, curl all endpoints, stop server). ✅

---

## Phase 1: Terraform integration (make terraform apply work)

Goal: `terraform init && terraform apply && terraform destroy` succeeds
using the official `azurerm` provider pointed at azemu.

| # | Task | File(s) | Status | Notes |
|---|------|---------|--------|-------|
| 1.1 | Add unhandled route logging middleware | `internal/middleware/unhandled.go` | DONE | `LogUnhandledRequests()` + thread-safe `UnhandledTracker` |
| 1.2 | Add `/api/unhandled` debug endpoint | `cmd/azemu/main.go` | DONE | Wired at `cmd/azemu/main.go:47` |
| 1.3 | Run `terraform init` against azemu, capture all requests | | DONE | Full `terraform apply && terraform destroy` cycle proven 2026-04-11 on `fix/metadata-classifier-bugs`. Five distinct blockers were uncovered and fixed in this branch (see TODO.md M1-M5 for the post-mortem). The flox manifest's `ARM_RESOURCE_MANAGER_ENDPOINT` workaround does NOT work on azurerm v4.68 — must use `ARM_METADATA_HOSTNAME=127.0.0.1:4567` instead. |
| 1.4 | Fix metadata response: add any missing fields provider expects | `internal/metadata/service.go` | DONE | Full canonical-schema rewrite against ground truth from `https://management.azure.com/metadata/endpoints?api-version=2022-09-01`. Every top-level field and every suffix entry now matches real Azure verbatim. Regression tests pin both shapes. |
| 1.5 | Fix token response: add `ext_expires_in`, `not_before`, `expires_on` fields | `internal/auth/token.go` | DEFERRED | Not blocking — `terraform apply` succeeds without it. Provider tolerates the current minimal token. |
| 1.6 | Fix provider registration: handle GET for specific resource types under a provider | `internal/arm/router.go` | DEFERRED | Not blocking the smoke flow. `azurerm_resource_group` works without it because `main.tf` sets `resource_provider_registrations = "none"`. |
| 1.7 | Handle subscription-level feature queries if provider calls them | `internal/arm/router.go` | DEFERRED | Not blocking. |
| 1.8 | Run `terraform apply`, fix failures iteratively | all `internal/arm/*`, `internal/metadata/*`, `internal/middleware/*` | DONE | Apply round-trip green; five blockers fixed (M1-M5 in TODO.md). |
| 1.9 | Run `terraform destroy`, verify clean | `internal/arm/router.go` | DONE | `listResourceGroupResources` handler unblocks polling; destroy round-trip green 2026-04-11. |
| 1.10 | Document all endpoints discovered during this phase in TODO.md | `TODO.md` | DONE | M1-M5 post-mortem table + Known Gaps section populated. |

Acceptance: `terraform apply -auto-approve && terraform destroy -auto-approve`
exits 0 with the `test/terraform/main.tf` config. ✅

**Structural improvements landed alongside Phase 1:**

- Persistent TLS cert via `AZEMU_CERT_PATH` (eliminates per-restart keychain trust friction). See `internal/auth/tls.go`.
- `internal/middleware/pathcase.go` `NormalizePath` for case-insensitive ARM path matching + `//` collapse.
- `.flox/env/manifest.toml` pinning Go, Terraform `^1.14`, pre-commit, jq, just, shellcheck, tflint with profile helpers (`azemu-start`, `tf-apply`, etc.) and an activation hook that installs `.git/hooks/pre-commit`.
- `.pre-commit-config.yaml` adopted from MiniBlue with hygiene + go vet/build + golangci-lint + markdownlint.
- `docs/SETUP.md` and `docs/TROUBLESHOOTING.md` published with the IPv6/`localhost` gotcha and the cert trust dance.

---

## Phase 2: Test coverage

Goal: comprehensive unit and integration tests, coverage targets met.

| # | Task | File(s) | Status | Notes |
|---|------|---------|--------|-------|
| 2.1 | Create test helpers: `httpGet`, `httpPut`, `httpDelete`, `assertStatus`, `assertJSONField` | `internal/arm/testutil_test.go` | DONE | Landed via `feat/vnet-subnet`. Ships `httpGet`/`httpGetRaw`/`httpPut`/`httpHead`/`httpDelete`/`assertStatus`/`decodeJSON`/`readBody` + `newTestServer` + `withAPIVersion` auto-injection. Uses `decodeJSON`+map assertions instead of `assertJSONField`. |
| 2.2 | Store tests: Put, Get, Delete, List, cascade delete, Export/Import round-trip, concurrent access | `internal/store/memory_test.go` | DONE | 11 tests, 100% coverage. Includes `TestMemoryStore_ConcurrentAccess` race scenario and `TestMemoryStore_Put_ReturnsNilError_Today` regression guard for the Phase 4 file-store transition. |
| 2.3 | ARM resource group tests: full CRUD, error cases, api-version, Azure error format | `internal/arm/rg_test.go` | DONE | 15 tests in `rg_test.go`, package coverage 90.2%. `TestRG_PUT_MissingLocation_CurrentlyAccepted` pins a pre-existing validation gap now tracked in `TODO.md`. |
| 2.4 | Auth tests: JWT claims, OIDC discovery fields, JWKS key match, token expiry | `internal/auth/token_test.go` | DONE | 14 new tests (9 from plan matrix + 5 coverage gap fillers), package coverage 88%. End-to-end JWKS signature verification pins the kid-in-header contract. |
| 2.5 | Metadata tests: all required fields present, URLs use correct host | `internal/metadata/service_test.go` | DONE | Landed via `fix/metadata-classifier-bugs`. 4 tests: required fields, all-localhost-urls-https, not-classified-as-azure-stack, dataplane-fields-are-https. The latter two pin the exact go-azure-sdk classifier conditions. |
| 2.6 | Middleware tests: api-version rejection, Azure headers, metadata exempt | `internal/middleware/azure_test.go` | DONE | 13 tests (8 from plan matrix + 5 for `unhandled.go`), 100% package coverage. `AzureHeaders` confirmed to always overwrite pre-existing headers. |
| 2.7 | Config tests: env var loading, defaults, flag overrides | `pkg/config/config_test.go` | TODO | |
| 2.8 | Integration smoke test: start server, full CRUD, verify responses | `test/integration/smoke_test.go` | PARTIAL | `test/integration/arm_test.go` from `feat/vnet-subnet` covers RG+VNet+Subnet full lifecycle through the production middleware stack (httptest in-process, not a real TCP listener). Subscriptions/providers/auth/metadata still uncovered. |
| 2.9 | Coverage report: verify targets from CLAUDE.md section 8 | | TODO | `go test -coverprofile` |

Acceptance: `go test ./... -v -race` passes. All packages meet coverage targets.

Subagent plan for this phase:

```text
Parallel subagents (3):
  A: test-writer for internal/store + internal/middleware
  B: test-writer for internal/arm (depends on test helpers from 2.1)
  C: test-writer for internal/auth + internal/metadata
Sequential after merge: integration test (2.8), coverage verification (2.9)
```

---

## Phase 3: Developer experience

Goal: wrapper CLI, terraform test support, improved Makefile.

| # | Task | File(s) | Status | Notes |
|---|------|---------|--------|-------|
| 3.1 | Create `scripts/aztf` wrapper script | `scripts/aztf` | TODO | Starts azemu if not running, exports env vars, passes args to terraform |
| 3.2 | Create `scripts/trust-cert.sh` helper | `scripts/trust-cert.sh` | TODO | macOS: security add-trusted-cert; Linux: update-ca-certificates |
| 3.3 | Create `.tftest.hcl` test file for resource groups | `test/terraform/main.tftest.hcl` | TODO | Terraform 1.6+ test framework |
| 3.4 | Update Makefile: `test`, `smoke`, `tf-test`, `coverage` targets | `Makefile` | TODO | |
| 3.5 | Add `--help` flag with usage text | `cmd/azemu/main.go` | TODO | Standard `flag` package |
| 3.6 | Print startup banner with version, ports, cert path, usage hint | `cmd/azemu/main.go` | TODO | |
| 3.7 | Update README.md with aztf usage, terraform test, full quick-start | `README.md` | TODO | |

Acceptance: `make tf-test` starts azemu, runs `terraform test`, stops azemu, exits 0.

---

## Phase 4: State management

Goal: file-based persistence, state export/import via CLI and HTTP API.

| # | Task | File(s) | Status | Notes |
|---|------|---------|--------|-------|
| 4.1 | Add CLI flags: `--port`, `--tls-port`, `--persist`, `--import`, `--export` | `cmd/azemu/main.go`, `pkg/config/config.go` | TODO | Standard `flag` package |
| 4.2 | Implement file-based store (`--persist` mode) | `internal/store/file.go` | TODO | Write-through to JSON file on every Put/Delete |
| 4.3 | Implement `--import` (load state from file on startup) | `cmd/azemu/main.go` | TODO | |
| 4.4 | Implement `--export` (dump current state to file, then exit) | `cmd/azemu/main.go` | TODO | |
| 4.5 | Add `GET /api/state/export` HTTP endpoint | `cmd/azemu/main.go` | TODO | Returns current state as JSON |
| 4.6 | Add `POST /api/state/import` HTTP endpoint | `cmd/azemu/main.go` | TODO | Replaces current state |
| 4.7 | Add `POST /api/state/reset` HTTP endpoint | `cmd/azemu/main.go` | TODO | Clears all state (useful for test isolation) |
| 4.8 | File store tests: write-through, reload, concurrent access | `internal/store/file_test.go` | TODO | |
| 4.9 | Integration test: persist, restart, verify state survives | `test/integration/persist_test.go` | TODO | |

Acceptance: azemu can persist state across restarts. `curl /api/state/export` returns valid JSON.
`curl -X POST /api/state/reset` clears all resources.

---

## Phase 5: Documentation and release prep

Goal: project is ready for public GitHub repo and first tagged release.

| # | Task | File(s) | Status | Notes |
|---|------|---------|--------|-------|
| 5.1 | Create `docs/PARITY.md` with Full/Stub/None matrix | `docs/PARITY.md` | TODO | |
| 5.2 | Create `docs/ARCHITECTURE.md` with extended design docs | `docs/ARCHITECTURE.md` | TODO | Include mermaid diagram from research report |
| 5.3 | Create `docs/CONTRIBUTING.md` | `docs/CONTRIBUTING.md` | TODO | How to add a resource, test requirements, PR checklist |
| 5.4 | Create `CHANGELOG.md` | `CHANGELOG.md` | TODO | Keep-a-changelog format |
| 5.5 | Create `TODO.md` with unimplemented endpoints and future work | `TODO.md` | TODO | Populated from Phase 1 discovery |
| 5.6 | Finalise README.md: badges, quick-start, examples, roadmap | `README.md` | TODO | |
| 5.7 | Add GitHub Actions CI workflow | `.github/workflows/ci.yml` | TODO | go vet, test, build, coverage |
| 5.8 | Add Dependabot config | `.github/dependabot.yml` | TODO | |
| 5.9 | Create `goreleaser.yml` for binary releases | `.goreleaser.yml` | TODO | macOS/Linux/Windows + Docker |
| 5.10 | Tag v0.1.0 | | TODO | |

Acceptance: `git tag v0.1.0`, CI passes, binary releases published, Docker image pushed.

---

## Future Phases (not in v0.1 scope, tracked for context)

### Phase 6: VNets + Subnets + DNS Zones

- **DONE (out-of-phase, `feat/vnet-subnet`):** ARM CRUD for `Microsoft.Network/virtualNetworks`, `Microsoft.Network/virtualNetworks/subnets`. Includes cascade delete via store prefix match, embedded-subnets-on-vnet-GET, `ParentResourceNotFound` on subnet PUT when parent vnet is missing, and 25 unit tests + 1 integration test. See `docs/PARITY.md`.
- TODO: ARM CRUD for `Microsoft.Network/dnsZones`, `Microsoft.Network/dnsZones/recordSets`
- TODO: Address space validation for VNets (current impl passes `addressSpace` through without CIDR/format checks)
- TODO: Inline subnets inside VNet PUT body are currently silently dropped; decide whether to honour them or keep the separate-subnet-PUT contract
- TODO: Auto SOA/NS for DNS zones

### Phase 7: Storage Accounts + Key Vault

- ARM management plane for `Microsoft.Storage/storageAccounts`
- Data plane for Key Vault secrets (CRUD)
- Correct endpoint suffixes in metadata response

### Phase 8: Identity (IMDS + ADO OIDC)

- IMDS token endpoint (`169.254.169.254` or configurable)
- Workload identity federation (issuer/subject/audience matching)
- Azure DevOps OIDC token issuer (compatible with `SYSTEM_OIDCREQUESTURI`)
- ADO service connection CRUD (minimal)

### Phase 9: Wrapper CLI (aztf v2)

- Go-based CLI replacing the shell script
- Auto-start azemu, cert trust, provider config generation
- `aztf snapshot save/load/list` for state management
- `aztf parity` to show supported resources

### Phase 10: Plugin SDK

- In-process Go plugin interface for resource modules
- Out-of-process gRPC/HTTP module server protocol
- Module registry and discovery
- Community module repo template

---

## How to use this file

1. Start at the current phase (check Status column).
2. Work through tasks in order within a phase (some can be parallelised with subagents).
3. Mark tasks DONE as they complete.
4. Do NOT start the next phase until the current phase's acceptance criteria are met.
5. If a task reveals new work, add it to the current phase with the next available number.
6. Keep this file updated. It is the single source of truth for project progress.
