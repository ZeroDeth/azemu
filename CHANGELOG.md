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
