# TASKS.md -- azemu Implementation Plan

Version: 0.1
Last updated: 2026-04-11
Status: Phase 1 + Phase 2 + Phase 3 acceptance MET. All per-package
coverage targets from `.claude/rules/tests.md` met or exceeded. Current
focus: Phase 2.5 (package-ownership cleanup) and Phase 4 (state management).

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

---

## Phase 2.5: Package ownership and response normalisation

Goal: clear the architectural follow-ups surfaced during Phase 2 before
Phase 4 introduces the file store. Small, well-scoped, reviewer-friendly.

| # | Task | File(s) | Status | Notes |
|---|------|---------|--------|-------|
| 2.5.1 | Move `OpenIDConfig` + `JWKS` mounts into `auth.TokenService.Routes` / `RoutesV2` so `internal/auth` owns its full public surface | `internal/auth/token.go`, `cmd/azemu/main.go`, `test/integration/harness_test.go` | TODO | Surfaced during `internal/auth/token_test.go` and the Phase 2.8 integration harness: both had to replicate the wiring from `cmd/azemu/main.go` verbatim. TODO.md "Known Gaps". |
| 2.5.2 | Decide on tags `null` vs `{}` normalisation for empty-tags responses | `internal/arm/*.go`, `docs/CONVENTIONS.md` | TODO | Matches existing RG behaviour today; real Azure returns `{}`. Either add a shared helper in `helpers.go` and update all responders, or document the choice in `docs/CONVENTIONS.md` S2 and leave as-is. TODO.md "Known Gaps". |

Acceptance: `internal/auth` exposes `Routes`/`RoutesV2` as the sole mount
points for the full token/OIDC/JWKS surface. Tags normalisation decision is
documented and (if chosen) implemented.

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
| 4.0 | Surface `store.Put` errors at every call site before the file store lands | `internal/arm/router.go`, `internal/arm/vnet.go`, `internal/arm/subnet.go` | TODO | Prerequisite. Every RG / VNet / Subnet handler currently does `_ = a.store.Put(id, res)` (or ignores the return entirely). Safe today because `MemoryStore.Put` cannot fail; **unsafe** the moment a file-backed store can return disk errors. Phase 4 cannot merge without this sweep or the first flaky disk write silently loses the resource. TODO.md "Known Gaps". |
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

## Phase 5: Documentation, governance, and release prep

Goal: project is ready for public GitHub repo, first external contributors,
and first tagged release. Governance files land alongside docs so a new
contributor sees a real open-source project, not a toy.

| # | Task | File(s) | Status | Notes |
|---|------|---------|--------|-------|
| 5.1 | Create `docs/PARITY.md` with Full/Stub/None matrix | `docs/PARITY.md` | TODO | Per-resource table with a link to the test that proves each Full claim |
| 5.2 | Create `docs/ARCHITECTURE.md` with extended design docs | `docs/ARCHITECTURE.md` | TODO | Include mermaid diagram from research report |
| 5.3 | Create `CONTRIBUTING.md` | `CONTRIBUTING.md` | TODO | How to add a resource, test requirements, PR checklist, link to the `add-resource` skill |
| 5.4 | Create `CHANGELOG.md` | `CHANGELOG.md` | TODO | Keep-a-changelog format; seed with Phase 1 and Phase 2 summaries |
| 5.5 | ~~Create `TODO.md`~~ | `TODO.md` | DONE | Populated during Phase 1 debugging; maintained ever since |
| 5.6 | Finalise README.md: badges, quick-start, examples, roadmap link | `README.md` | TODO | Quick-start uses docker-compose; link `ROADMAP.md` above the fold |
| 5.7 | Add GitHub Actions CI workflow | `.github/workflows/ci.yml` | TODO | `go vet`, `go test -race -coverprofile`, `go build`, integration suite under `-tags=integration`, markdownlint, golangci-lint |
| 5.8 | Add Dependabot config | `.github/dependabot.yml` | TODO | go modules + github-actions + docker |
| 5.9 | Create `goreleaser.yml` for binary releases | `.goreleaser.yml` | TODO | macOS/Linux/Windows + Docker image push to ghcr.io |
| 5.10 | Create `CODE_OF_CONDUCT.md` | `CODE_OF_CONDUCT.md` | TODO | Contributor Covenant v2.1, standard template, contact email |
| 5.11 | Create `SECURITY.md` | `SECURITY.md` | TODO | Supported versions, how to report a vulnerability, expected response time. Private channel (email or GitHub security advisories) not public issues |
| 5.12 | Create `RELEASING.md` | `RELEASING.md` | TODO | Release steps: tag, run `goreleaser`, update `CHANGELOG.md`, write the release notes, announce. The checklist the maintainer follows so nothing is tribal knowledge |
| 5.13 | Create `CODEOWNERS` | `.github/CODEOWNERS` | TODO | One owner per package until contributors grow; keeps PR reviewer routing explicit |
| 5.14 | Create `.github/ISSUE_TEMPLATE/bug_report.md` and `feature_request.md` | `.github/ISSUE_TEMPLATE/*.md` | TODO | Bug report template MUST ask for azemu version, azurerm version, terraform version, full error output, and whether `/api/unhandled` shows anything. These four questions short-circuit 90% of M1-M5-class triage. |
| 5.15 | Create `.github/PULL_REQUEST_TEMPLATE.md` | `.github/PULL_REQUEST_TEMPLATE.md` | TODO | Pre-filled checklist: tests added, docs updated, parity matrix updated if resource-level, changelog entry drafted |
| 5.16 | Add `renovate.json` (or stick with Dependabot) | `renovate.json` | TODO | Decide one or the other, not both |
| 5.17 | Tag v0.1.0 | | TODO | Blocked on 5.1 through 5.16 |

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
| 6.2 | `azurerm_public_ip` | `Microsoft.Network/publicIPAddresses` | TODO | Prerequisite for LB and Application Gateway. Allocation mode, SKU, IP version. |
| 6.3 | `azurerm_network_security_group` + rules | `Microsoft.Network/networkSecurityGroups` | TODO | Rules stored as children, cascade delete on NSG delete. Attach-to-subnet wiring via `networkSecurityGroup.id` reference on subnet body. |
| 6.4 | `azurerm_lb` + backend pool + rule + probe | `Microsoft.Network/loadBalancers` | TODO | The "Load Balancer" from the ROADMAP roster. Children modelled as path-extensions of the LB id so cascade delete works. |
| 6.5 | `azurerm_application_gateway` | `Microsoft.Network/applicationGateways` | TODO | The "ingress" primitive. Minimal config: frontend IP, backend pool, HTTP listener, routing rule. |
| 6.6 | `azurerm_dns_zone` + record sets (A, AAAA, CNAME, TXT, MX, SRV, NS, SOA) | `Microsoft.Network/dnsZones` + `.../{type}` | TODO | Auto-SOA and auto-NS on zone create. Record sets as children. |
| 6.7 | Address-space validation for VNets | `internal/arm/vnet.go` | TODO | Reject invalid CIDR, reject overlapping prefixes inside the same VNet. Currently `addressSpace` is passed through unvalidated. |
| 6.8 | Inline subnets inside VNet PUT body: honour or keep dropping | `internal/arm/vnet.go`, `docs/PARITY.md` | TODO | Decision already documented in `TODO.md` "Known Gaps". Pick one and make it explicit in PARITY. |

### Phase 6.5: Use-case scenarios for v0.2

Goal: `examples/terraform/scenarios/` grows with each resource batch so
new contributors learn by running real configurations, not toy snippets.
A scenario that does not run green in CI is deleted.

| # | Task | Scenario | Status | Requires |
|---|---|---|---|---|
| 6.5.1 | `scenarios/three-tier/` | Web + app + data tier with LB, App Gateway, VNet + 3 Subnets, NSG, Public IP | TODO | 6.2 through 6.5 |
| 6.5.2 | `scenarios/static-site/` | Storage account hosting a static site behind a CDN profile with a DNS zone | TODO | Phase 7 (Storage, CDN) and 6.6 (DNS) |
| 6.5.3 | `scenarios/dns-with-records/` | DNS zone plus A/AAAA/CNAME/TXT/MX record sets | TODO | 6.6 |

### Phase 7: Storage, Key Vault, CDN (v0.2)

Goal: the secrets-and-state story. Storage account management + container
creation, Key Vault secrets CRUD, CDN profile + endpoint, correct suffix
entries in the metadata response.

| # | Task | ARM provider | Status | Notes |
|---|---|---|---|---|
| 7.1 | `azurerm_storage_account` | `Microsoft.Storage/storageAccounts` | TODO | Management plane. Name uniqueness check across subscription. |
| 7.2 | `azurerm_storage_container` | `.../storageAccounts/blobServices/containers` | TODO | Minimal blob data-plane surface for what `azurerm_storage_container` actually writes. Not a full `azcopy` target (see ROADMAP non-goals). |
| 7.3 | `azurerm_key_vault` | `Microsoft.KeyVault/vaults` | TODO | Management plane. Access policies as children. |
| 7.4 | `azurerm_key_vault_secret` | `...vaults/secrets` | TODO | Secrets data plane. Versioned. |
| 7.5 | `azurerm_cdn_profile` + `azurerm_cdn_endpoint` | `Microsoft.Cdn/profiles` + `.../endpoints` | TODO | The "CDN" from the ROADMAP roster. |
| 7.6 | Verify `suffixes.*` in metadata response still match go-azure-sdk expectations for Storage/KV/CDN | `internal/metadata/service.go` | TODO | Add regression tests for the three suffix families. |

### Phase 8: Identity, AKS, Azure DevOps bridge (v0.3)

Goal: Terraform inside an Azure DevOps pipeline, using workload identity
federation, provisions an AKS cluster (stub) and a Managed Identity against
azemu with zero cloud cost. See ROADMAP v0.3 and the non-goals section.

| # | Task | ARM provider / endpoint | Status | Notes |
|---|---|---|---|---|
| 8.1 | `azurerm_user_assigned_identity` | `Microsoft.ManagedIdentity/userAssignedIdentities` | TODO | Prerequisite for federated identity credentials. |
| 8.2 | `azurerm_federated_identity_credential` | `.../userAssignedIdentities/{name}/federatedIdentityCredentials` | TODO | issuer/subject/audience matching. The machinery workload identity needs. |
| 8.3 | IMDS token endpoint | `169.254.169.254/metadata/identity/oauth2/token` (host binding optional) | TODO | Pair with 8.2. |
| 8.4 | `azurerm_kubernetes_cluster` | `Microsoft.ContainerService/managedClusters` | TODO (Stub only) | Management plane only. No live Kubernetes control plane. Explicit non-goal. |
| 8.5 | Azure DevOps OIDC issuer endpoint | `SYSTEM_OIDCREQUESTURI` compatible | TODO | New package `internal/ado/`. |
| 8.6 | ADO service connection CRUD | `dev.azure.com/{org}/{project}/_apis/serviceendpoint/endpoints` | TODO | Minimal surface the `azuredevops` Terraform provider hits during workload-identity flows. |
| 8.7 | `scenarios/aks-workload/` | — | TODO | RG + VNet + Subnet + AKS + Managed Identity + Key Vault. |
| 8.8 | `scenarios/ado-pipeline/` | — | TODO | ADO service connection + workload identity federation + Key Vault + Storage. |

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
