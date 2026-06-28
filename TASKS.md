# TASKS.md -- azemu Implementation Plan

Version: 0.1
Last updated: 2026-06-28
Status: Phases 0 through 9 are shipped. v0.1.0 tagged 2026-04-21; v0.3.0
released 2026-06-28. Scenario 8.7.1 (the server-less OTA delivery design)
shipped in PR #77: an ADO pipeline signs an update manifest with a Key Vault
key and writes immutable artefacts to Blob, a release pipeline promotes by a
server-side blob copy, and a CDN serves the static files. This needed one
generic azemu capability (a CDN content data plane); it shipped with the
`ota-delivery` scenario.
Current focus: lifting the azurerm provider-version pins. static-site is
pinned `< 4.35` pending a Front Door migration; the storage scenarios are
pinned `< 4.35` pending host-style `*.blob.core.windows.net` routing. Both
gaps are tracked in TODO.md Known Gaps.

> **Ready-for-testing / scenario-CI health (2026-06-27, PR #74 merged).** The
> Terraform Scenarios CI job had been red for weeks. The fail-fast loop in
> `make tf-test-scenarios` masked it: only the first failing scenario ran.
> Removing the masking exposed the systemic bugs, now all fixed and merged to
> `main`:
>
> - **M9** (the real destroy-hang root cause): the metadata `resourceManager`
>   endpoint carried a trailing slash, so the provider built DELETE URIs as
>   `//subscriptions/...`; the hashicorp/go-azure-sdk delete poller then GETs
>   the parent list (`200`) forever instead of the resource (`404`) and hung
>   ~30 min per resource. Dropping the slash fixes it.
> - **M7**: an `operationresults` endpoint plus absolute
>   `Azure-AsyncOperation`/`Location` headers. These are ignored by the SDK for
>   DELETE (it skips the async-operation poller), so M7 was a red herring for
>   the hang; the endpoint remains for non-delete LRO polling.
> - **M8**: inline `azurerm_lb_probe`/`lb_rule` management dropped by `putLB`;
>   now persisted as child resources and reconciled (upsert listed, delete
>   stale when the array key is present, untouched when omitted).
> - **M6**: azurerm version drift broke `storage_container`; scenarios pinned
>   `>= 4.0, < 4.35` with `init -upgrade` dropped.
>
> All six scenarios round-trip `terraform apply` + `destroy` against the real
> `hashicorp/azurerm` provider, verified locally and by the Terraform Scenarios
> CI job on the PR branch (Terraform cannot run in the dev container itself,
> since the agent proxy blocks the provider registry). PR #74 squash-merged to
> `main` as `114a333`, which re-runs the same green tree.

<!-- MD028: HTML comment separates adjacent blockquotes. -->

> **Strategy, non-goals, and the per-release resource roster live in
> `ROADMAP.md`.** `TASKS.md` is the execution ledger and `ROADMAP.md` is
> the north star. If they ever disagree, `ROADMAP.md` wins; this file
> gets updated to match.

<!-- MD028: HTML comment separates adjacent blockquotes. -->

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
| 2.7 | Config tests: env var loading, defaults, flag overrides | `pkg/config/config_test.go` | DONE | 11 cases, 100% coverage. Pins AZEMU_SUBSCRIPTION_ID / AZEMU_TENANT_ID / AZEMU_METADATA_HOST / AZEMU_CERT_PATH, the empty-string-is-unset semantics of `envOr`, and the "ports are hardcoded today" contract. |
| 2.8 | Integration smoke test: start server, full CRUD, verify responses | `test/integration/*.go` | DONE | `arm_test.go` (RG+VNet+Subnet lifecycle), `auth_test.go` (token mint + OIDC discovery + JWKS end-to-end signature verification), `metadata_test.go` (M1/M2/M3 canonical-schema regression pins). Shared TLS harness in `harness_test.go` mirrors `cmd/azemu/main.go`. |
| 2.9 | Coverage report: verify targets from `.claude/rules/tests.md` | | DONE | `store` 100%, `arm` 92.6%, `auth` 88.0%, `metadata` 100%, `middleware` 100%, `config` 100%. All packages meet or exceed targets. `go test ./... -race` green. |

Acceptance: `go test ./... -v -race` passes. All packages meet coverage targets. ✅

**Phase 2 closeout batch (2026-04-11) also landed:**

- Deleted `azureTimestamp` dead code from `internal/arm/router.go` (was 0% coverage, never called).
- Fixed `putResourceGroup` empty-location validation gap so RG PUT matches
  the vnet/subnet pattern; `TestRG_PUT_MissingLocation_CurrentlyAccepted`
  flipped to `TestRG_PUT_MissingLocation_Returns400` plus a whitespace
  companion. Removed from `TODO.md` Known Gaps.
- Backfilled the three deferred VNet/Subnet coverage holes
  (`headSubnet` 77.8% → 100%, `deleteSubnet` 81.8% → 100%, `writeVNetList`
  85.7% → 100%).

Subagent plan that shipped Phase 2:

```text
Parallel subagents (3):
  A: test-writer for internal/store + internal/middleware
  B: test-writer for internal/arm (depends on test helpers from 2.1)
  C: test-writer for internal/auth + internal/metadata
Sequential closeout batch: pkg/config tests (2.7), integration auth+metadata
(2.8), coverage verification (2.9), VNet/Subnet backfill, cleanup commits.
```

**Phase 2 secondary coverage pass (2026-05-24, PR #57):** Filled gaps in
packages that were 0% or low coverage after the main Phase 2 batch. Plan:
`docs/plans/2026-05-24-001-test-coverage-gaps-plan.md`.

| # | Task | File(s) | Status | Notes |
|---|------|---------|--------|-------|
| 2.U1 | `cmd/azemu` pure-logic tests | `cmd_purelogic_test.go` | DONE | stringSlice, credentialMatches, formatUptime, statusIcon, setEnvDefaults, resolveCertFile, tlsInsecureConfig, insecureHTTPClient, snapshotDir |
| 2.U2 | `cmd/azemu` HTTP helper tests | `cmd_http_test.go` | DONE | probeHealth / waitForHealth via httptest |
| 2.U3 | `cmd/azemu` FIC resolver tests | `ficresolver_test.go` | DONE | ResolveFederatedIdentity with in-memory store |
| 2.U4 | `internal/ado` gap tests | `serviceconnection_test.go` | DONE | endpointBelongsToProject 33%→100%, writeADOJSON encode-failure path |
| 2.U5 | `internal/arm` gap tests | `dns_test.go`, `federated_identity_credential_test.go`, `keyvault_secret_test.go` | DONE | DNS property passthrough, FIC validation errors, KV secret attribute passthrough |

Total: 78.3% overall (up from 75.6%), 568 tests pass with `-race`.

---

## Phase 2.5: Package ownership and response normalisation

Goal: clear the architectural follow-ups surfaced during Phase 2 before
Phase 4 introduces the file store. Small, well-scoped, reviewer-friendly.

| # | Task | File(s) | Status | Notes |
|---|------|---------|--------|-------|
| 2.5.1 | Move `OpenIDConfig` + `JWKS` mounts into `auth.TokenService.TenantRoutes` so `internal/auth` owns its full public surface | `internal/auth/token.go`, `cmd/azemu/main.go`, `test/integration/harness_test.go` | DONE | New `TenantRoutes(chi.Router)` method mounts oauth2 + OIDC + JWKS under one `/{tenantID}` group. `main.go` and `harness_test.go` each reduced to a single `r.Route("/{tenantID}", tokenSvc.TenantRoutes)` call. |
| 2.5.2 | Normalise empty tags to `{}` in all ARM responses | `internal/arm/router.go`, `internal/arm/vnet.go`, `docs/CONVENTIONS.md` | DONE | `normaliseTags()` helper in `router.go` converts nil to `map[string]string{}`. Applied in `putResourceGroup` and `putVNet`. Decision documented in `docs/CONVENTIONS.md` "Tags normalisation". Pinned by `TestRG_PUT_NilTags_NormalisedToEmptyObject` and `TestVNet_PUT_NilTags_NormalisedToEmptyObject`. |

Acceptance: `internal/auth` exposes `TenantRoutes` as the sole mount point
for the full token/OIDC/JWKS surface. Tags normalisation to `{}` is
implemented via `normaliseTags()` and documented in `docs/CONVENTIONS.md`. ✅

---

## Phase 3: Developer experience

Goal: first-run onboarding is one command. Docker, docker-compose, a Nix
flake, a bootstrap `examples/terraform/` directory, and Makefile polish.
The flox environment stays as the contributor-side workspace; Docker is
what new users hit first.

| # | Task | File(s) | Status | Notes |
|---|------|---------|--------|-------|
| 3.1 | Create `scripts/aztf` wrapper script | `scripts/aztf` | DONE | Detects running azemu via `docker compose ps`; starts it if absent; exports `SSL_CERT_FILE` + `ARM_*` env vars; execs `terraform "$@"`. Passes shellcheck. |
| 3.2 | Create `scripts/trust-cert.sh` helper | `scripts/trust-cert.sh` | DONE | macOS: `security add-trusted-cert`; Linux: `update-ca-certificates`. Optional; default cert story uses `SSL_CERT_FILE` instead. |
| 3.3 | Create `.tftest.hcl` test file | `examples/terraform/main.tftest.hcl` | DONE | Terraform 1.6+ native test. One `run "full_lifecycle"` block with assertions on all three output IDs. Moved to `examples/terraform/` (not `test/terraform/`). |
| 3.4 | Update Makefile: `tf-test`, `coverage`, `docker-compose`, `docker-compose-down` targets | `Makefile` | DONE | Also added `-ldflags "-X main.Version=$(VERSION)"` to `build` target. |
| 3.5 | Add `--help` flag with usage text | `cmd/azemu/main.go` | DONE | stdlib `flag` package. Prints banner, env var table, port layout. `--version` also added. |
| 3.6 | Print startup banner with version, ports, cert path | `cmd/azemu/main.go` | DONE | Linker-overridable `var Version = "dev"`. Banner to stderr before zerolog. |
| 3.7 | Rewrite README.md with docker-compose quick-start | `README.md` | DONE | Docker path is now the default; flox kept as contributor workflow. Links `ROADMAP.md` above the fold. |
| 3.8 | Hardened `Dockerfile` | `Dockerfile` | DONE | Multi-stage Go build, alpine runtime with `wget` for healthcheck, `VOLUME /azemu`, env defaults for `AZEMU_CERT_PATH` and `AZEMU_METADATA_HOST`, `EXPOSE 4566 4567 4568`. |
| 3.9 | `docker-compose.yml` for single-node local use | `docker-compose.yml` | DONE | Exposes 4566/4567/4568, bind-mounts `./.azemu:/azemu`, healthcheck via `wget http://localhost:4568/health`. |
| 3.10 | `flake.nix` for Nix users who do not use flox | `flake.nix` | DONE | `buildGoModule` with `subPackages = [ "cmd/azemu" ]`, `devShells.default` with go and terraform. Flox remains the contributor workflow. |
| 3.11 | `examples/terraform/` bootstrap configs | `examples/terraform/*.tf`, `examples/terraform/README.md` | DONE | One file per resource (RG + VNet + Subnet), `provider.tf` with inline mock credentials, `variables.tf`, `outputs.tf`. README shows the `docker compose up` -> `terraform apply` -> `terraform destroy` loop. |
| 3.12 | Add `GET /health` HTTP endpoint (non-TLS) on `:4568` | `cmd/azemu/main.go`, `pkg/config/config.go` | DONE | Separate plain-HTTP `http.Server` on configurable `HealthPort` (default 4568). Returns `{"status":"ok","version":"...","uptime_seconds":N}`. No middleware, no TLS. |

Acceptance: `docker compose up -d --build && export SSL_CERT_FILE=$PWD/.azemu/cert-bundle.pem && cd examples/terraform && terraform init && terraform apply -auto-approve && terraform destroy -auto-approve` exits 0 on a fresh clone with no flox and no manual cert trust. The one `SSL_CERT_FILE` export is required because Go's TLS stack needs to trust azemu's self-signed cert; `scripts/aztf` automates this for users who prefer zero env exports. ✅

**Structural improvements landed alongside Phase 3:**

- Cert bundle file mode changed from `0600` to `0644` in `internal/auth/tls.go` so Docker bind-mounts are readable by the host user.
- `HealthPort` field added to `pkg/config/config.go` (default 4568), following the existing `HTTPPort`/`HTTPSPort` pattern.
- `.gitignore` updated to cover `coverage.html`.
- `docs/SETUP.md` updated with a Docker quick-start section and expanded make-targets table.

---

## Phase 4: State management

Goal: file-based persistence, state export/import via CLI and HTTP API.

| # | Task | File(s) | Status | Notes |
|---|------|---------|--------|-------|
| 4.0 | Surface `store.Put` errors at every call site before the file store lands | `internal/arm/router.go`, `internal/arm/vnet.go`, `internal/arm/subnet.go` | DONE | All three handlers now check `store.Put` errors and return 500 with Azure error format on failure. Landed in the pre-Phase-4 hardening commit alongside store copy semantics, auth error propagation, writeJSON buffer-first, and middleware singleton removal. |
| 4.1 | Add CLI flags: `--persist`, `--import`, `--export` | `cmd/azemu/main.go`, `pkg/config/config.go` | DONE | `--persist` also via `AZEMU_PERSIST_PATH` env var. Port flags (`--port`, `--tls-port`) deferred. |
| 4.2 | Implement file-based store (`--persist` mode) | `internal/store/file.go` | DONE | `FileStore` wraps `MemoryStore` with write-through persistence. Atomic writes via tmp+rename. 10 tests, 92% coverage. |
| 4.3 | Implement `--import` (load state from file on startup) | `cmd/azemu/main.go` | DONE | Reads file, calls `state.Import()`, continues serving. Works with both memory and file stores. |
| 4.4 | Implement `--export` (dump current state to file, then exit) | `cmd/azemu/main.go` | DONE | Calls `state.Export()`, writes file, exits with code 0. |
| 4.5 | Add `GET /api/state/export` HTTP endpoint | `cmd/azemu/main.go` | DONE | Returns full state as JSON. |
| 4.6 | Add `POST /api/state/import` HTTP endpoint | `cmd/azemu/main.go` | DONE | Replaces current state from request body. |
| 4.7 | Add `POST /api/state/reset` HTTP endpoint | `cmd/azemu/main.go` | DONE | Calls `state.Reset()`. Added `Reset()` to Store interface. |
| 4.8 | File store tests: write-through, reload, concurrent access | `internal/store/file_test.go` | DONE | 10 tests: write-through, reload, timestamps, delete, reset, tmp cleanup, missing file, corrupt file, import, concurrent. |
| 4.9 | Integration test: persist, restart, verify state survives | `test/integration/persist_test.go` | DONE | Shipped in PR #44. |

Acceptance: azemu can persist state across restarts. `curl /api/state/export` returns valid JSON.
`curl -X POST /api/state/reset` clears all resources.

---

## Phase 5: Documentation, governance, and release prep

Goal: project is ready for public GitHub repo, first external contributors,
and first tagged release. Governance files land alongside docs so a new
contributor sees a real open-source project, not a toy.

| # | Task | File(s) | Status | Notes |
|---|------|---------|--------|-------|
| 5.1 | Create `docs/PARITY.md` with Full/Stub/None matrix | `docs/PARITY.md` | DONE | Proof column links every Full row to its test. Shipped in PR #9. |
| 5.2 | Create `docs/ARCHITECTURE.md` with extended design docs | `docs/ARCHITECTURE.md` | DONE | Mermaid request-flow diagram added. Shipped in PR #9. |
| 5.3 | Create `CONTRIBUTING.md` | `CONTRIBUTING.md` | DONE | Ground rules, dev env, add-resource walkthrough, test bar, PR checklist. |
| 5.4 | Create `CHANGELOG.md` | `CHANGELOG.md` | DONE | Keep-a-changelog format; Phase 1-4 entries. Shipped in PR #9. |
| 5.5 | ~~Create `TODO.md`~~ | `TODO.md` | DONE | Populated during Phase 1 debugging; maintained ever since |
| 5.6 | Finalise README.md: badges, quick-start, examples, roadmap link | `README.md` | DONE | Roadmap checklist updated, Phase 3/4 flipped. Shipped in PR #9. |
| 5.7 | Add GitHub Actions CI workflow | `.github/workflows/ci.yml` | DONE | Lint (golangci-lint, markdownlint), test (-race -coverprofile), build + verify. |
| 5.8 | Add Dependabot config | `.github/dependabot.yml` | DONE | gomod weekly, github-actions weekly, docker monthly. |
| 5.9 | Create `goreleaser.yml` for binary releases | `.goreleaser.yml` | DONE | macOS/Linux/Windows (amd64+arm64), Docker to ghcr.io, release workflow on tag push. |
| 5.10 | Create `CODE_OF_CONDUCT.md` | `CODE_OF_CONDUCT.md` | DONE | References Contributor Covenant v2.1, contact email, enforcement summary. |
| 5.11 | Create `SECURITY.md` | `SECURITY.md` | DONE | Supported versions, private reporting via GH advisories or email, 48h ack SLA. |
| 5.12 | Create `RELEASING.md` | `RELEASING.md` | DONE | Pre-release checks, changelog prep, tag, goreleaser, post-release verify. |
| 5.13 | Create `CODEOWNERS` | `.github/CODEOWNERS` | DONE | @ZeroDeth owns everything; per-package lines for future granularity. |
| 5.14 | Create `.github/ISSUE_TEMPLATE/bug_report.md` and `feature_request.md` | `.github/ISSUE_TEMPLATE/*.md` | DONE | Bug template asks azemu/azurerm/terraform versions, error output, /api/unhandled. |
| 5.15 | Create `.github/PULL_REQUEST_TEMPLATE.md` | `.github/PULL_REQUEST_TEMPLATE.md` | DONE | Checklist: tests, pre-commit, parity, changelog, no unapproved deps. |
| 5.16 | Add `renovate.json` (or stick with Dependabot) | `.github/dependabot.yml` | DONE | Chose Dependabot: simpler for a single-maintainer project, no external service. |
| 5.17 | Tag v0.1.0 | | DONE | Tagged 2026-04-21. Release workflow triggers goreleaser. |

Acceptance: `git tag v0.1.0`, CI passes, binary releases published, Docker image pushed to `ghcr.io/zerodeth/azemu:v0.1.0`, `ROADMAP.md` and `CONTRIBUTING.md` both linked from `README.md` above the fold.

---

## Future Phases

Resource rosters and fidelity targets for these phases are driven by
`ROADMAP.md`. This section only tracks the concrete file-level tasks.

### Phase 6: Networking extended (v0.2)

Goal: every networking primitive a real three-tier web app needs. VNet + Subnet landed out-of-phase in v0.1; v0.2 fills in the rest.

| # | Task | ARM provider | Status | Notes |
|---|---|---|---|---|
| 6.1 | ~~VNet + Subnet ARM CRUD~~ | `Microsoft.Network/virtualNetworks` + `.../subnets` | DONE | Shipped out-of-phase in v0.1 on `feat/vnet-subnet`. 25 unit tests + integration coverage. |
| 6.2 | `azurerm_public_ip` | `Microsoft.Network/publicIPAddresses` | DONE | Static/Dynamic alloc, SKU, fake `ipAddress` assigned on creation and preserved on update. 15 unit tests + integration test. |
| 6.3 | `azurerm_network_security_group` + rules | `Microsoft.Network/networkSecurityGroups` | DONE | Security rules as child resources under NSG id prefix; cascade delete; embedded in NSG GET/LIST. 30 unit tests + integration test. NSG-to-subnet wiring deferred (tracked in TODO). |
| 6.4 | `azurerm_lb` + backend pool + rule + probe | `Microsoft.Network/loadBalancers` | DONE | Backend pools, rules, and probes as child resources under LB id prefix; cascade delete; embedded in LB GET/LIST. SKU at top level (same pattern as Public IP). ~45 unit tests + integration test. |
| 6.5 | `azurerm_application_gateway` | `Microsoft.Network/applicationGateways` | DONE | Monolithic PUT (no child endpoints); SKU with name/tier/capacity at top level; all inline sub-config arrays preserved verbatim; operationalState: Running. 17 unit tests + integration test. |
| 6.6 | `azurerm_dns_zone` + record sets (A, AAAA, CNAME, TXT, MX, SRV, NS, SOA) | `Microsoft.Network/dnsZones` + `.../{type}` | DONE | Auto-SOA + auto-NS on zone create. Single `{recordType}` chi param handles all 8 record types. 40 unit tests + integration test covering cascade delete, FQDN computation, numberOfRecordSets, and multi-type round-trip. |
| 6.7 | Address-space validation for VNets | `internal/arm/vnet.go` | DONE | `validateAddressPrefixes` rejects invalid CIDRs and overlapping prefixes with 400 `InvalidAddressPrefix`. Missing `addressSpace` is still accepted (backward compat). 5 new unit tests. |
| 6.8 | Inline subnets inside VNet PUT body: honour or keep dropping | `internal/arm/vnet.go`, `docs/PARITY.md` | DONE | Decision: keep dropping. Avoids split-brain between inline data and child store entries. Documented in PARITY.md notes and pinned by `TestVNet_PUT_InlineSubnets_DroppedSilently`. |

### Phase 6.5: Use-case scenarios for v0.2

Goal: `examples/terraform/scenarios/` grows with each resource batch so
new contributors learn by running real configurations, not toy snippets.
A scenario that does not run green in CI is deleted.

| # | Task | Scenario | Status | Requires |
|---|---|---|---|---|
| 6.5.1 | `scenarios/three-tier/` | Web + app + data tier with LB, App Gateway, VNet + 3 Subnets, NSG, Public IP | DONE | 6.2 through 6.5 |
| 6.5.2 | `scenarios/static-site/` | Storage account hosting a static site behind a CDN profile with a DNS zone | DONE | Phase 7 (Storage, CDN) and 6.6 (DNS) |
| 6.5.3 | `scenarios/dns-with-records/` | DNS zone plus A/AAAA/CNAME/TXT/MX record sets | DONE | 6.6 |

### Phase 7: Storage, Key Vault, CDN (v0.2)

Goal: the secrets-and-state story. Storage account management + container
creation, Key Vault secrets CRUD, CDN profile + endpoint, correct suffix
entries in the metadata response.

| # | Task | ARM provider | Status | Notes |
|---|---|---|---|---|
| 7.1 | `azurerm_storage_account` | `Microsoft.Storage/storageAccounts` | DONE | Management plane. Name uniqueness check across subscription. SKU/kind at top level. `primaryEndpoints` returns Azurite path-style URLs (blob :10000, queue :10001, table :10002) derived from `AZEMU_AZURITE_ENDPOINT`. `POST listkeys` returns Azurite dev key. |
| 7.2 | `azurerm_storage_container` | `.../storageAccounts/blobServices/containers` | DONE | Blob containers as child resources under account id prefix. Parent existence check. Cascade delete when account is deleted. |
| 7.3 | `azurerm_key_vault` | `Microsoft.KeyVault/vaults` | DONE | Management plane. `vaultUri` computed as `https://{name}.vault.azure.net/`. SKU/soft-delete defaults. 18 unit tests + integration test. |
| 7.4 | `azurerm_key_vault_secret` | `...vaults/secrets` | DONE | Secrets data plane on `/keyvault/{name}/secrets`. `vaultUri` in vault response rewritten to `AZEMU_KV_ENDPOINT/keyvault/{name}/`. Versioned (each PUT creates new UUID version). Cascade delete when vault is deleted. 14 unit tests. |
| 7.5 | `azurerm_cdn_profile` + `azurerm_cdn_endpoint` | `Microsoft.Cdn/profiles` + `.../endpoints` | DONE | CDN profile with SKU at top level; endpoint `hostName` computed as `{name}.azureedge.net`; cascade delete; parent-existence check. ~25 unit tests. |
| 7.6 | Verify `suffixes.*` in metadata response still match go-azure-sdk expectations for Storage/KV/CDN | `internal/metadata/service.go` | DONE | `TestMetadata_CanonicalSuffixNames` pins `storage: "core.windows.net"` and `keyVaultDns: "vault.azure.net"`. CDN uses ARM endpoints directly, no suffix entry needed. |
| 7.7 | `azurerm_redis_cache` (Standard tier) + `redis` sidecar | `Microsoft.Cache/Redis` | DONE | CRUD + HEAD + LIST + listKeys; SKU validation inside `properties.sku` (Basic/Standard/Premium, families C/P, capacity ranges); Premium-only fields rejected on Basic/Standard; deterministic dev keys whose primary matches the sidecar `--requirepass`; `hostName` derived from `AZEMU_REDIS_ENDPOINT`. ADR 0003 promoted to Implemented. Out of scope: Premium clustering/persistence/geo-replication, `regenerateKey`, TLS-wrapped 6380. |
| 7.8 | Confirm `redisCache: "redis.cache.windows.net"` suffix in metadata response | `internal/metadata/service.go` | DONE | Added to `internal/metadata/service.go` suffixes block; regression assertion in `TestMetadata_CanonicalSuffixNames`. |
| 7.9 | Mirror ADR 0003 to `website/docs/resources/design-decisions/0003-add-azure-cache-for-redis.md` and add nav entry to `website/mkdocs.yml` | `website/docs/resources/design-decisions/`, `website/mkdocs.yml` | DONE | File matches `docs/adr/0003-add-azure-cache-for-redis.md` verbatim; nav entry in `website/mkdocs.yml`; `Status: Implemented` with `Implemented: 2026-04-28`. |

### Phase 8: Identity, AKS, Azure DevOps bridge (v0.3)

Goal: Terraform inside an Azure DevOps pipeline, using workload identity
federation, provisions an AKS cluster (stub) and a Managed Identity against
azemu with zero cloud cost. See ROADMAP v0.3 and the non-goals section.

| # | Task | ARM provider / endpoint | Status | Notes |
|---|---|---|---|---|
| 8.1 | `azurerm_user_assigned_identity` | `Microsoft.ManagedIdentity/userAssignedIdentities` | DONE | Deterministic `principalId`/`clientId` via `uuid.NewSHA1` for stable Terraform round-trips. 17 unit tests. |
| 8.2 | `azurerm_federated_identity_credential` | `.../userAssignedIdentities/{name}/federatedIdentityCredentials` | DONE | issuer/subject/audience matching implemented. Token endpoint accepts `client_assertion`, matches it against stored FIC rules for the requested identity `client_id`, and mints an azemu access token honoured by the Key Vault secrets data plane. Unit + integration coverage added. |
| 8.3 | IMDS token endpoint | `/metadata/identity/oauth2/token` (mounted before `/metadata` in chi) | DONE | RS256 JWT; `Metadata: true` header enforced; `expires_in` as string per IMDS contract. 7 unit tests. |
| 8.4 | `azurerm_kubernetes_cluster` + agent pools | `Microsoft.ContainerService/managedClusters` | DONE | Management plane only. Default k8s version 1.29.0; computed fqdn; cascade-delete node pools on cluster delete. ~30 unit tests. |
| 8.5 | Azure DevOps OIDC issuer endpoint | `SYSTEM_OIDCREQUESTURI` compatible; plain HTTP on `:4569` | DONE | New package `internal/ado/`. Own RSA-2048 key independent of TokenService. `/.well-known/openid-configuration` + `/discovery/keys` + OIDC token endpoint. 10 unit tests. |
| 8.6 | ADO service connection CRUD | `/{org}/{project}/_apis/serviceendpoint/endpoints` | DONE | In-memory store with `sync.RWMutex`. Auto-assigns UUID. `isReady: true`, `owner: "Library"` on create/update. Name-filter on list. 14 unit tests. |
| 8.7 | `scenarios/aks-workload/` | — | DONE | RG + VNet + Subnet + AKS (3-node) + User-Assigned Identity + Key Vault + Secret. |
| 8.7.1 | `scenarios/ota-delivery/` server-less OTA delivery (Blob + Key Vault sign + CDN) | `examples/terraform/scenarios/ota-delivery/`, `internal/arm/cdn_dataplane.go` | DONE | Shipped in PR #77 (`af5031c`). Reframed from the kind/AKS-workload hybrid to a server-less, static-file delivery design with no compute on the read path: a build pipeline signs an Expo Updates Protocol v1 manifest with a Key Vault key (existing key data plane) and writes immutable artefacts to Blob; a release pipeline promotes by a server-side blob copy and writes `rollout.json`; a CDN fronts the Blob origin. Needed one generic azemu capability, a CDN content data plane (reverse-proxy to the Blob origin with origin-header passthrough on `{endpoint}.azureedge.net`), which also upgrades `static-site`. The kind/CSI/expo-open-ota path in [ADR 0002](../docs/adr/0002-azemu-plus-kind-for-aks-workload-deployments.md) is deferred; this design avoids a second runtime. CI runs the ARM-half tftest; `make ota-delivery` runs the full publish + serve-assert loop locally. |
| 8.9 | Mirror ADR 0002 to `website/docs/resources/design-decisions/0002-azemu-plus-kind-for-aks-workload-deployments.md` and add nav entry to `website/mkdocs.yml` | `website/docs/resources/design-decisions/`, `website/mkdocs.yml` | DONE | Landed in PR #42 alongside website mermaid fixes. Status kept as `Proposed` until 8.7.1 ships. |
| 8.8 | `scenarios/ado-pipeline/` | — | DONE | ARM resources for ADO pipeline: managed identity + federated credential + Key Vault + secret + Storage + blob container. ADO service connection example via curl in README. |

### Phase 9: Multi-toolchain CLI (`azemu` subcommands)

Goal: the `azemu` binary itself becomes the unified CLI. No separate wrapper
binary. Each toolchain adapter auto-starts the emulator, injects the correct
env vars and config, and execs the underlying tool. Replaces `scripts/aztf`.

| # | Task | Status | Notes |
|---|------|--------|-------|
| 9.1 | `azemu serve` subcommand (current bare-start behaviour) | DONE | Refactored `cmd/azemu/` into subcommand dispatch. `main.go` is thin dispatcher; `serve.go` holds all server logic. No-arg and legacy flag syntax both default to serve. |
| 9.2 | `azemu tf <args>` (Terraform adapter) | DONE | Auto-start via background `azemu serve`, health-poll up to 30 s, cert resolution (AZEMU_CERT_PATH / .azemu/ / /tmp/), `ARM_*` + `SSL_CERT_FILE` env injection, `syscall.Exec` to terraform. |
| 9.3 | `azemu pulumi <args>` (Pulumi adapter) | DONE | `ARM_*` + `ARM_ENDPOINT` env injection, auto-start, exec pulumi. |
| 9.4 | `azemu kubectl <args>` (Kubernetes adapter) | DONE | `AZURE_*` env injection for azure-identity, auto-start, exec kubectl. |
| 9.5 | `azemu python <args>` (Python Azure SDK adapter) | DONE | `AZURE_*` + `AZURE_AUTHORITY_HOST` + `AZURE_ARM_URL` + `REQUESTS_CA_BUNDLE` env injection, auto-start, exec python. |
| 9.6 | `azemu parity` (show supported resources) | DONE | Embedded parity matrix; tabwriter table output; `--json` flag for machine-readable output. |
| 9.7 | `azemu snapshot save\|load\|list` (state management) | DONE | save/load/list/reset subcommands; snapshots stored in `~/.azemu/snapshots/`; wraps `/api/state/export`, `/api/state/import`, `/api/state/reset`. |
| 9.8 | `azemu status` (health and version check) | DONE | Probes health endpoint; shows version, status, uptime; exits 0/1 for scripting. |
| 9.9 | Remove `scripts/aztf` shell wrapper | DONE | Deleted; all doc references updated to `azemu tf`. |

Acceptance: `azemu tf apply` from a cold start (no running emulator) exits 0
against the `examples/terraform/` config. `azemu pulumi preview` works with
the Pulumi Azure Native provider pointed at azemu. ✅

### Phase 10: Plugin SDK

- In-process Go plugin interface for resource modules
- Out-of-process gRPC/HTTP module server protocol
- Module registry and discovery
- Community module repo template

### Phase 11: Helm chart + Kubernetes deploy (v0.2 nice-to-have)

Intentionally NOT v0.1. A chart is worth shipping only once azemu emulates
enough resources to justify a team-shared CI cluster running it.

- `charts/azemu/Chart.yaml`
- `charts/azemu/templates/deployment.yaml` (one replica for file-store mode)
- `charts/azemu/templates/service.yaml`
- `charts/azemu/templates/pvc.yaml` (backing store for `AZEMU_CERT_PATH` and the Phase 4 file store)
- `charts/azemu/values.yaml`
- `examples/kubernetes/` manifests for users who do not want Helm

---

## How to use this file

1. Start at the current phase (check Status column).
2. Work through tasks in order within a phase (some can be parallelised with subagents).
3. Mark tasks DONE as they complete.
4. Do NOT start the next phase until the current phase's acceptance criteria are met.
5. If a task reveals new work, add it to the current phase with the next available number.
6. Keep this file updated. It is the single source of truth for project progress.
