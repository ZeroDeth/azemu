<!-- markdownlint-disable MD024 -->
<!--
  MD024 (no-duplicate-heading) is disabled for this file because the
  Keep a Changelog format reuses the same section names ("Added",
  "Changed", "Fixed", ...) under each release. The duplication is
  intentional and the markdownlint-cli command-line flags do not
  support a siblings_only override; an inline directive is the
  conventional fix recommended by the markdownlint maintainers for
  changelog files. See:
  https://github.com/DavidAnson/markdownlint/blob/main/doc/md024.md
-->

# Changelog

All notable changes to azemu will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- Load balancer probes and load balancing rules now round-trip. They have no
  standalone ARM create operation, so the azurerm provider (`azurerm_lb_probe`,
  `azurerm_lb_rule`) writes them inline via the parent Load Balancer PUT, but
  `putLB` dropped those inline arrays, so the provider saw the probe vanish
  after apply (`Provider produced inconsistent result after apply: ... Root
  object was present, but now absent`). `putLB` now persists inline
  `probes`/`loadBalancingRules` as child entries that `getLB` embeds. The
  upsert is additive, so a later probe-less `azurerm_lb` PUT cannot wipe them.
  See TODO.md M8.
- Async DELETE polling now resolves instead of hanging. Every resource's
  `202 Accepted` DELETE set a `Location: /subscriptions/{sub}/operationresults/{id}`
  header, but nothing served that path, so the azurerm provider polled a dead
  URL until its 30-minute delete timeout (`polling after Delete: context
  deadline exceeded`) and the relative URL also failed the older go-autorest
  CDN poller outright (`StatusCode=0`). New `internal/arm/operations.go` adds
  the `operationresults` endpoint (returns `{"status":"Succeeded"}`; azemu
  deletes synchronously) and builds an absolute `Location` carrying the
  request's `api-version`. Each DELETE advertises the operation via both
  `Azure-AsyncOperation` (which the azurerm poller prefers and which expects
  the `{"status":"Succeeded"}` body the endpoint returns) and `Location`. This
  affected every async-delete resource (NSG, LB and children, CDN, subnet,
  DNS zone, VNet, AKS, Redis, App Gateway, Public IP, resource group). See
  TODO.md M7.
- AKS: `POST .../managedClusters/{name}/listClusterUserCredential` and
  `listClusterAdminCredential` are now implemented, returning a kubeconfig
  that the azurerm provider parses into `kube_config` /
  `kube_admin_config`. Previously both fell through to the unhandled-route
  handler and its 501 NotImplemented failed every
  `azurerm_kubernetes_cluster` apply (aks-workload scenario).
- static-site scenario: azurerm pinned to `>= 4.0, < 4.35`. From v4.35.0
  the provider blocks creating classic CDN resources after the 2025-10-01
  deprecation date (client-side wall-clock check, no opt-out), failing the
  scenario before any request reaches azemu. Migration to Front Door is
  tracked in TODO.md.
- All Terraform scenarios (and the top-level `examples/terraform`) now pin
  azurerm to `>= 4.0, < 4.35`, and `make tf-test*` no longer passes
  `-upgrade`. Previously `init -upgrade` pulled the newest azurerm on every
  run, so CI silently drifted onto provider versions azemu was never
  validated against. azurerm 4.78+ added an `azurerm_storage_container`
  account-ID check requiring a `core.windows.net` blob-endpoint suffix,
  which azemu's Azurite path-style endpoints (per ADR 0001) do not satisfy;
  that broke the ado-pipeline scenario. Pinning makes scenario CI
  deterministic. See TODO.md M6.
- CI: `make tf-test-scenarios` now runs every scenario and reports a
  pass/fail summary instead of aborting on the first failure. The old
  fail-fast loop hid the status of every scenario alphabetically after the
  first broken one.

### Added (Key Vault keys, sign-only RSA)

- Key Vault keys data plane: create/import RSA keys (2048/3072/4096) with
  versioning, public-JWK GET, list and list-versions, PATCH updates,
  delete with cascade, and the `sign` operation (RS256, RSASSA-PKCS1-v1_5
  over a SHA-256 digest), including the versionless form that resolves the
  current key version. Signatures verify against the returned public JWK.
  (`azurerm_key_vault_key`)
- Host-based Key Vault data-plane routing: `vaultUri` is now
  `https://{vault}.vault.localhost[:port]/` and root-level `/keys` and
  `/secrets` routes resolve the vault from the Host header. The azurerm
  provider requires both the `{name}.vault.**` host shape
  (`KeyVaultIDFromBaseUrl`) and vault-less nested-item URLs
  (`ParseNestedItemID`); the previous path-style ids broke
  `azurerm_key_vault_secret` and `azurerm_key_vault_key` read-back.
  Path-style routes under `/keyvault/{vault}/` remain for raw clients.
- Subscription-wide Resources list (`GET /subscriptions/{sub}/resources`)
  with `$filter=resourceType eq '...'` support; used by the provider to
  map a vaultUri back to the vault ARM ID.
- Key Vault soft-delete purge stubs: `POST .../deletedvaults/{name}/purge`
  (the shape `vaults.VaultsClient#PurgeDeleted` actually sends) and
  data-plane `DELETE /deleted{keys,secrets}/{name}` returning 204.
- Storage `blobServices/default` PUT: the `blob_properties` block on
  `azurerm_storage_account` no longer fails with 405; properties round-trip
  on GET.
- `AZURITE_ACCOUNTS` pre-registration in `docker-compose.yml`
  (`devstoreaccount1`, `examplestorage001`, `azemuotasa`) plus a SETUP.md
  section on registering Terraform-chosen storage account names.
- `examples/terraform/scenarios/ota-updates/`: OTA update pipeline scenario
  (storage account + key vault + RSA signing key) with a documented
  publish-time sign call.
- `azurerm_key_vault_key` example in `examples/terraform/keyvault.tf` with
  a `key_vault_key_id` output and test assertion.

### Changed (Key Vault keys, sign-only RSA)

- The self-signed TLS certificate now carries a `*.vault.localhost` SAN.
  Existing persisted bundles are regenerated automatically on startup and
  must be trusted again (`security add-trusted-cert ...` on macOS; see
  docs/TROUBLESHOOTING.md).
- Key Vault nested-item ids (secret `id`, key `kid`) moved from
  `{kvEndpoint}/keyvault/{vault}/...` to
  `https://{vault}.vault.localhost[:port]/...`.

### Added (Phase 7.7: Azure Cache for Redis)

- `Microsoft.Cache/Redis` CRUD + HEAD + list-by-RG + list-by-sub.
  SKU validated inside `properties.sku` (Basic, Standard, Premium with
  family C/P and capacity ranges); Premium-only properties (shardCount,
  subnetId, RDB/AOF persistence keys) rejected on Basic/Standard.
  (`azurerm_redis_cache`)
- `POST .../Microsoft.Cache/Redis/{name}/listKeys`: returns deterministic
  dev keys (`azemu-dev-primary-key`, `azemu-dev-secondary-key`). The
  primary value matches the Redis sidecar's `--requirepass` so SDK clients
  authenticated via the ARM response succeed against the data plane.
- `AZEMU_REDIS_ENDPOINT` env var (default `redis://azemu-redis:6379`).
  azemu derives the `hostName` field on Redis ARM responses from the URL
  host so callers connect to the configured sidecar instead of
  `redis.cache.windows.net`.
- `docker-compose.yml`: optional `redis` service (`redis:7-alpine`) on the
  `redis` profile so default users see no extra container; healthcheck via
  `redis-cli ping` with the dev password.
- `redisCache: "redis.cache.windows.net"` suffix in `/metadata/endpoints`,
  pinned by `TestMetadata_CanonicalSuffixNames`.
- `examples/terraform/redis_cache.tf` plus `redis_cache_id` /
  `redis_cache_hostname` outputs and a `terraform test` assertion.
- `docs/adr/0003-add-azure-cache-for-redis.md` promoted from Proposed to
  Implemented (Implemented date: 2026-04-28).
- `website/docs/resources/design-decisions/0002-...md` and `0003-...md`
  mirrors plus mkdocs nav entries (closes the website-mirror gap from
  TODO.md for ADRs 0002 and 0003).
- `docs/SETUP.md` and `website/docs/reference/setup.md`: new
  `AZEMU_REDIS_ENDPOINT` env-var row and a "Redis sidecar (optional)"
  section explaining the compose profile, `--requirepass` contract, and
  host-mode setup.

### Added (Phase 2 secondary coverage pass)

- `cmd/azemu` package now has test coverage (was 0%). New test files cover:
  pure-logic helpers (`stringSlice`, `credentialMatches`, `formatUptime`,
  `statusIcon`, `setEnvDefaults`, `resolveCertFile`, `tlsInsecureConfig`,
  `insecureHTTPClient`, `snapshotDir`), HTTP helpers (`probeHealth`,
  `waitForHealth`), and `ResolveFederatedIdentity`.
- `internal/ado`: `endpointBelongsToProject` coverage 33%→100%;
  `writeADOJSON` encode-failure path covered.
- `internal/arm`: DNS `dnsZoneResponse` property passthrough, FIC
  validation-error paths, Key Vault secret attribute passthrough and
  custom-attribute round-trip.
- Overall test coverage: 75.6%→78.3%. 568 tests pass with `-race`.

### Fixed

- `test/integration/` build resurrected. `arm.NewRouter` had grown a
  second parameter in Phase 7 (KeyVaultEndpoint) without updating the
  integration harness; this PR adds the missing endpoints (Azurite, Key
  Vault, Redis) to both `buildFullServer` and `buildProductionLikeServer`
  and corrects two pre-existing assertions that compared against
  real-Azure hostnames (`integrationacct.blob.core.windows.net`,
  `mytestvault.vault.azure.net`) instead of the configured test
  endpoints.

### Added (Phase 7: Storage, Key Vault)

- `Microsoft.Storage/storageAccounts` CRUD + HEAD + list-by-RG + list-by-sub.
  Name uniqueness check across subscription. SKU/kind at top level; soft-delete
  and access-tier defaults. (`azurerm_storage_account`)
- `Microsoft.Storage/storageAccounts/blobServices/containers` CRUD + HEAD + list.
  Parent-existence check; cascade delete when account is deleted.
  (`azurerm_storage_container`)
- `POST .../storageAccounts/{name}/listKeys`: returns Azurite's well-known
  development account key so SDK clients authenticate against the Azurite
  sidecar without extra configuration.
- `primaryEndpoints` block in storage account responses now returns path-style
  Azurite endpoint URLs (blob `:10000`, queue `:10001`, table `:10002`) derived
  from `AZEMU_AZURITE_ENDPOINT`.
- `AZEMU_AZURITE_ENDPOINT` env var (default `http://azurite:10000`). azemu
  derives queue and table base URLs from this single knob.
- `docker-compose.yml`: `azurite` service (`mcr.microsoft.com/azure-storage/azurite`)
  with ports 10000-10002, named volume, healthcheck, and `depends_on`
  (condition: `service_healthy`) so azemu starts only after Azurite is ready.
- `Microsoft.KeyVault/vaults` CRUD + HEAD + list-by-RG + list-by-sub.
  `vaultUri` computed as `https://{name}.vault.azure.net/`; SKU defaults to
  `standard`; soft-delete defaults to 90 days. (`azurerm_key_vault`)
- `docs/adr/0001-delegate-storage-data-plane-to-azurite.md`: Architecture
  Decision Record capturing the Azurite delegation decision and its
  rationale, alternatives, and consequences. Status: Implemented.
- `docs/SETUP.md`: Storage and Azurite section; `AZEMU_AZURITE_ENDPOINT` in
  the env-var table; Azurite port table.

## [v0.1.0] - 2026-04-21

### Added (Phase 5: governance and CI)

- `CONTRIBUTING.md` with ground rules, dev environment, add-resource
  walkthrough, test requirements, and PR checklist.
- `CODE_OF_CONDUCT.md` referencing Contributor Covenant v2.1.
- `SECURITY.md` with supported versions, private reporting channels
  (GitHub Security Advisories, email), and 48h acknowledgement SLA.
- `RELEASING.md` with the full release checklist (pre-release, changelog,
  tag, goreleaser, post-release verification).
- `.github/CODEOWNERS` with @ZeroDeth as default owner.
- `.github/ISSUE_TEMPLATE/bug_report.md` asking for azemu/azurerm/terraform
  versions, full error output, and `/api/unhandled` output.
- `.github/ISSUE_TEMPLATE/feature_request.md`.
- `.github/PULL_REQUEST_TEMPLATE.md` with pre-filled checklist.
- `.github/workflows/ci.yml` with lint (golangci-lint, markdownlint), test
  (`go test -race -coverprofile`), and build jobs. Triggers on push/PR to main.
- `.github/workflows/release.yml` triggering goreleaser on tag push.
- `.github/dependabot.yml` for gomod (weekly), github-actions (weekly),
  and docker (monthly).
- `.goreleaser.yml` building macOS/Linux/Windows (amd64+arm64) binaries and
  Docker image to `ghcr.io/zerodeth/azemu`.
- `.golangci.yml` excluding `fmt.Fprint*` and `(io.ReadCloser).Close` from
  errcheck.
- `.markdownlint-cli2.jsonc` disabling MD013, MD033, MD041.
- `docs/PARITY.md` Proof column linking every Full row to its test file.
- `docs/ARCHITECTURE.md` mermaid request-flow diagram.

### Added (Phase 1-4)

- `.claude/agents/*.md` — five frontmatter-driven subagent definitions
  (`arm-resource-implementer`, `test-writer`, `code-reviewer`,
  `terraform-compatibility-debugger`, `docs-writer`). Claude Code auto-delegates
  when a task description matches; previously these roles lived as prose recipes
  in `docs/SUBAGENTS.md` and had to be hand-copied into Task tool invocations.
  Per <https://code.claude.com/docs/en/sub-agents>.
- `.claude/skills/*/SKILL.md` — four slash-invokable playbooks
  (`/add-resource`, `/modify-store`, `/validate-terraform`, `/before-commit`).
  `before-commit` carries `disable-model-invocation: true` so Claude never
  auto-runs the full validation sequence. Per
  <https://code.claude.com/docs/en/skills>.
- `.gitignore` negations for `.claude/agents/` and `.claude/skills/` so the
  new directories are version-controlled alongside the existing
  `.claude/rules/` exception.

### Changed

- `docs/SUBAGENTS.md` renamed to `docs/ORCHESTRATION.md` and trimmed to the
  three multi-agent composition patterns (parallel resource implementation,
  test-then-fix, coverage push). The five role definitions moved to
  `.claude/agents/*.md`.
- `docs/CHECKLISTS.md` replaced with an 18-line redirect table pointing at
  the new skills. Existing content moved verbatim into the four skill files.
- `AGENTS.md` "Subagents and orchestration" section rewritten to document the
  new `.claude/agents/` and `.claude/skills/` directories and the
  `/before-commit` / `/validate-terraform` / `/add-resource` / `/modify-store`
  slash invocations. Project-files table updated to match.
- `.claude/rules/arm-handlers.md`, `.claude/rules/tests.md`,
  `.claude/rules/docs.md`, and `docs/CONVENTIONS.md` updated to reference the
  new skill paths instead of the old `docs/CHECKLISTS.md` and
  `docs/SUBAGENTS.md` locations.

### Fixed

- `CLAUDE.md` referenced a machine-local auto-memory file
  (`~/.claude/projects/.../memory/feedback_claude_md_steering.md`) that only
  existed on the maintainer's machine. Anthropic's own memory docs state auto
  memory is machine-local and not shared across machines, so any contributor
  cloning the repo on a fresh machine would hit a dangling reference. The
  Anthropic source quotes and refactor history now live inline in the
  `CLAUDE.md` HTML maintainer comment, which is stripped before context
  injection (zero session-token cost) and travels with the repo.
  Per <https://code.claude.com/docs/en/memory>.

### Added

- `Dockerfile` with a multi-stage Go build, alpine runtime, `wget` for
  healthchecks, `VOLUME /azemu`, env defaults for `AZEMU_CERT_PATH` and
  `AZEMU_METADATA_HOST`, and `EXPOSE 4566 4567 4568`.
- `docker-compose.yml` for single-node local use: exposes `4566`/`4567`/`4568`,
  bind-mounts `./.azemu:/azemu`, healthcheck via
  `wget http://localhost:4568/health`.
- `GET /health` plain-HTTP endpoint on a configurable `HealthPort` (default
  `4568`). Returns `{"status":"ok","version":"...","uptime_seconds":N}`.
  No TLS and no middleware so container probes stay boring.
- Startup banner to stderr with version, ports, and cert path. Linker-
  overridable `var Version = "dev"` via `-ldflags "-X main.Version=$(VERSION)"`.
- `--help` and `--version` stdlib-`flag` handling in `cmd/azemu/main.go`
  with an env-var table and port layout.
- `scripts/aztf` wrapper that detects a running azemu via
  `docker compose ps`, starts it if absent, exports `SSL_CERT_FILE` and the
  `ARM_*` variables, and execs `terraform "$@"`. Shellcheck-clean.
- `scripts/trust-cert.sh` helper (macOS `security add-trusted-cert`, Linux
  `update-ca-certificates`). Optional; the default path uses
  `SSL_CERT_FILE` instead.
- `examples/terraform/` bootstrap config (RG + VNet + Subnet across
  `provider.tf` / `variables.tf` / `outputs.tf`) plus
  `examples/terraform/main.tftest.hcl` native Terraform 1.6+ test with one
  `run "full_lifecycle"` block.
- `flake.nix` for Nix users outside flox: `buildGoModule` for `cmd/azemu`
  and `devShells.default` with go + terraform.
- Makefile targets: `tf-test`, `coverage`, `docker-compose`,
  `docker-compose-down`, plus `-ldflags "-X main.Version=..."` on `build`.
- `docs/SETUP.md` Docker quick-start section and an expanded make-targets
  table.
- File-backed state store (`internal/store/file.go`). `FileStore` wraps
  `MemoryStore` with write-through persistence via atomic `tmp + rename`.
- `--persist` CLI flag (also `AZEMU_PERSIST_PATH` env var) that activates
  `FileStore`. `--import` loads state at startup; `--export` dumps current
  state to a file and exits `0`.
- `GET /api/state/export` returns full state as JSON.
- `POST /api/state/import` replaces current state from the request body.
- `POST /api/state/reset` clears all resources. `Store.Reset()` added to
  the interface so memory + file stores both implement it.
- 10 `internal/store/file_test.go` tests covering write-through, reload,
  timestamps, delete, reset, tmp cleanup, missing file, corrupt file,
  import, and concurrent access.

### Changed

- Cert bundle file mode in `internal/auth/tls.go` relaxed from `0600` to
  `0644` so Docker bind-mounts are readable by the host user when the
  container writes the file.
- `pkg/config/config.go` grew a `HealthPort` field (default `4568`) next
  to `HTTPPort`/`HTTPSPort`.
- `.gitignore` now covers `coverage.html`.
- Pre-Phase-4 hardening (7 critical + 9 high issues from Go review):
  all `store.Put` call sites (RG, VNet, Subnet) now surface errors and
  return `500` with Azure error format on failure; store copy semantics
  fixed so callers cannot mutate stored resources; `writeJSON` switched
  to buffer-first so a failed encode no longer produces a half-written
  body; auth errors propagate instead of being swallowed; middleware
  singletons removed.

### Fixed

- `azureTimestamp` dead code in `internal/arm/router.go` (0% coverage,
  never called) deleted during the Phase 2 closeout batch.
- `putResourceGroup` now rejects empty or whitespace-only `location`
  with `400 InvalidRequestContent`, matching the vnet/subnet pattern.
  Pinned by `TestRG_PUT_MissingLocation_Returns400` and
  `TestRG_PUT_WhitespaceOnlyLocation_Returns400`.
- `headSubnet`/`deleteSubnet`/`writeVNetList` coverage gaps backfilled
  to 100% via `TestSubnet_HEAD_NotFound_Returns404_EmptyBody`,
  `TestSubnet_DELETE_NotFound_Returns404`, and
  `TestVNet_LIST_ByRG_FiltersOutSubnets`. `internal/arm` package
  coverage climbed from 90.7% to 92.6%.

### Added

- Virtual Networks (`Microsoft.Network/virtualNetworks`) ARM CRUD + HEAD with
  cascade-delete and child-subnet embedding on GET.
- Subnets (`Microsoft.Network/virtualNetworks/subnets`) ARM CRUD + HEAD with
  parent-vnet existence check (returns `404 ParentResourceNotFound`).
- `internal/middleware/pathcase.go` `NormalizePath` middleware that lowercases
  known ARM literal segments (case-insensitive) and collapses double slashes.
  Wired into the router before `RequireAPIVersion` so real azurerm camelCase
  paths reach lowercase chi routes.
- `listResourceGroupResources` handler (`GET /subscriptions/.../resourceGroups/{rg}/resources`)
  returning `{"value": []}` so `terraform destroy` can poll an empty RG without
  hitting `/api/unhandled` and surfacing a misleading "internal-error".
- `AZEMU_CERT_PATH` config option and `auth.LoadOrGenerateSelfSignedTLS`: when
  set, azemu loads or generates a persistent PEM bundle (cert + EC private key,
  mode `0600`) so contributors trust the self-signed cert in their keychain
  once and can restart the binary freely.
- `internal/arm/testutil_test.go` shared test helpers (`newTestServer`,
  `withAPIVersion`, `httpPut`/`httpGet`/`httpHead`/`httpDelete`, `decodeJSON`).
- 4 metadata regression tests pinning canonical field/suffix names, the
  `IsAzureStack` classifier conditions, and the all-HTTPS data plane invariant.
- 8 path-normalization regression tests covering the exact azurerm camelCase
  path strings, OAuth path passthrough, and double-slash collapse.
- 4 RG `resources` listing tests (empty, populated, RG self-exclusion, OData).
- `.flox/env/manifest.toml` pinning Go, Terraform `^1.14`, just, jq, shellcheck,
  tflint, pre-commit. Profile defines `azemu-start`/`azemu-stop`/`azemu-status`
  and `tf-init`/`tf-plan`/`tf-apply`/`tf-destroy` aliases. Activation hook
  installs the project pre-commit hook on first run.
- `.pre-commit-config.yaml` with trailing-whitespace, end-of-file-fixer,
  check-yaml/json, mixed-line-ending, no-commit-to-branch=main,
  tekwizely/pre-commit-golang `go-fmt`/`go-vet-repo-mod`/`go-build-repo-mod`,
  `golangci-lint` v1.62.2 and `markdownlint-cli` v0.42.0.
- `docs/SETUP.md` and `docs/TROUBLESHOOTING.md` covering provider redirection,
  cert trust on macOS/Linux, and the IPv6/`localhost` resolution gotcha.
- `docs/ARCHITECTURE.md`, `docs/CONVENTIONS.md`, `docs/CHECKLISTS.md`, and
  `docs/SUBAGENTS.md` extracted from the previous monolithic `CLAUDE.md`.
- `.claude/rules/arm-handlers.md`, `.claude/rules/go-style.md`,
  `.claude/rules/tests.md`, and `.claude/rules/docs.md` — path-scoped rule
  files that load only when Claude Code is editing matching files, per the
  mechanism documented at <https://code.claude.com/docs/en/memory>.

### Changed

- **`CLAUDE.md` refactored from 643 lines to 43 lines** to match Anthropic's
  published guidance ("target under 200 lines per CLAUDE.md file"). The file
  is now a thin wrapper that imports `AGENTS.md` via the `@` directive and
  adds a handful of Claude-Code-specific overrides. Code blocks, ARM contract
  tables, auth fidelity rules, per-package coverage targets, and workflow
  checklists moved to `docs/CONVENTIONS.md`, `docs/CHECKLISTS.md`, and the
  `.claude/rules/*.md` path-scoped files.
- **`AGENTS.md` refactored from 215 lines to 116 lines** and promoted to the
  primary "README for agents" (<https://agents.md> cross-vendor spec).
  Subagent role definitions and orchestration patterns moved to
  `docs/SUBAGENTS.md`. `AGENTS.md` now contains project identity, build/test
  commands, convention pointers, branch discipline, and safety rules.
- Per-session steering context reduced from ~643 lines (just `CLAUDE.md`) to
  159 lines (`CLAUDE.md` + imported `AGENTS.md`), a 75% reduction in the
  context tokens consumed at session start.

- `internal/metadata/service.go` rewritten against the canonical Azure schema
  from `https://management.azure.com/metadata/endpoints?api-version=2022-09-01`.
  Field names now match real Azure verbatim (`portal`, `graph`,
  `appInsightsResourceId`, `attestationResourceId`, `synapseAnalyticsResourceId`,
  `logAnalyticsResourceId`, `ossrDbmsResourceId`, `suffixes.storage`,
  `suffixes.keyVaultDns`, `suffixes.storageSyncEndpointSuffix`, ...) so
  `go-azure-sdk` can build per-service authorizers without falling through to
  the Azure Stack rejection path.
- ARM port `:4566` now serves HTTPS (was HTTP) so the `azurerm` provider does
  not classify the environment as Azure Stack via the `resourceManager` URL
  scheme check. Both ports share the same self-signed certificate.
- `cmd/azemu/main.go` now starts both servers with a shared TLS config and
  wires `NormalizePath` before `RequireAPIVersion`. Cert lifecycle messages
  distinguish "generated and persisted" vs "loaded from existing bundle".

### Fixed

- M1: Azure Stack rejection caused by `dataPlane` URLs declared as `http://`.
  Switched to `https://` and pinned by `TestMetadata_DataPlaneFieldsAreHTTPS`.
- M2: Azure Stack rejection caused by `authentication.tenant` being a UUID;
  the `IsAzureStack` classifier requires the literal string `"common"`.
- M3: Storage authorizer build failure caused by hand-rolled metadata field
  names that did not match Azure's canonical schema.
- M4: chi v5 case-sensitivity mismatch where azurerm sent camelCase
  `resourceGroups` and azemu's routes were registered as lowercase
  `resourcegroups`. Resolved structurally by `NormalizePath`.
- M5: `terraform destroy` polling loop misreported a 501 from the missing
  RG resources list endpoint as a generic internal-error.
- `.gitignore` `azemu` pattern was matching `cmd/azemu/` as a directory
  wildcard, so `cmd/azemu/main.go` had never been tracked. Removed the bare
  pattern; the file is now in version control.

## [0.0.1] - 2026-04-09

### Added

- Project scaffold: dual HTTP/HTTPS server, chi routing, zerolog logging
- Metadata service (`/metadata/endpoints`) for azurerm provider redirection
- Mock OAuth2 token endpoint with RS256 JWT signing
- OIDC discovery and JWKS endpoints
- ARM facade: subscriptions, provider registration, resource group CRUD
- Azure-compatible middleware: response headers, api-version enforcement
- In-memory state store with export/import
- Self-signed TLS certificate generation (ECDSA P-256)
- Dockerfile (multi-stage Go build)
- Makefile with build, run, test, docker, smoke targets
- Example Terraform config (`test/terraform/main.tf`)
- CLAUDE.md, AGENTS.md, TASKS.md for AI agent orchestration
- docs/PARITY.md resource compatibility matrix
