---
title: "test: Fill coverage gaps — cmd/azemu, internal/ado, internal/arm"
type: test
status: completed
created: 2026-05-24
---

## Context

Phase 2 (TASKS.md) met all coverage targets for `internal/*` packages. The
`cmd/azemu` package was never covered — it is `package main` and requires a
separate test strategy. Several helper functions in `internal/ado` and
`internal/arm` also sit below target thresholds. This plan addresses all gaps
in priority order: most impactful first.

**Baseline:** `go test ./... -cover` reports 75.6% total. The `cmd/azemu`
package contributes 0% and drags the aggregate down by ~8 points.

---

## Scope

**In scope:**

- Unit tests for pure-logic functions in `cmd/azemu` (package main tests)
- HTTP-level tests for `probeHealth` / `waitForHealth` using `httptest`
- Unit tests for `ResolveFederatedIdentity` using the in-memory store
- Gap-filling tests in `internal/ado` and `internal/arm` below their targets

**Out of scope:**

- Testing `runServe`, `runTF`, `runKubectl`, `runPulumi`, `runPython` — these
  exec external binaries and are integration-level concerns
- Testing `execBinary` / `startAzemuBackground` — they use `syscall.Exec` /
  `os.StartProcess` and require a running binary
- Adding new resources or changing the code under test

---

## Key Technical Decisions

**`package main` test approach.** Go allows `*_test.go` files in `package main`
in the same directory. These test internal (unexported) functions directly.
Tests for `cmd/azemu` go in `cmd/azemu/*_test.go` with the `package main`
declaration — no `_test` suffix on the package name.

**No new dependencies.** Standard `testing` + `net/http/httptest` only, per
`.claude/rules/tests.md`.

**Coverage target for `cmd/azemu`.** The conventions file sets no explicit target
for `cmd/azemu` because it wasn't there when Phase 2 shipped. Aim for 60%+ on
the package: enough to cover all pure-logic paths and HTTP helper paths, without
forcing tests on os-process-level code.

---

## Implementation Units

### U1. `cmd/azemu` pure-logic unit tests

**Goal:** Cover `credentialMatches`, `stringSlice`, `formatUptime`, `statusIcon`,
`setEnvDefaults`, and `resolveCertFile` — all zero-dependency pure functions.

**Requirements:** Bring `cmd/azemu` coverage from 0% to at least ~40% with no
infrastructure. These are the safest functions to test and highest bang-for-effort.

**Dependencies:** None.

**Files:**

- `cmd/azemu/cmd_purelogic_test.go` (new)

**Approach:**

- Declare `package main` in the test file to access unexported functions.
- Use table-driven tests for every function that has ≥3 cases.
- `setEnvDefaults` tests must save/restore env vars around each case using
  `t.Setenv` (Go 1.17+) so tests don't pollute each other.
- `resolveCertFile` tests use `t.TempDir()` to create real files on disk; test
  the three cases: config path exists, config path absent but cwd file exists,
  no file exists (returns configPath fallback or empty).

**Patterns to follow:** Style mirrors `pkg/config/config_test.go` for env-var
manipulation tests.

**Test scenarios:**

- `TestStringSlice_stringSlice`: `[]string` passthrough, `[]interface{}` with
  string items, `[]interface{}` with mixed types (non-string dropped), nil input
  returns nil/empty
- `TestCredentialMatches_issuerMismatch`: props with wrong issuer → false
- `TestCredentialMatches_subjectMismatch`: correct issuer, wrong subject → false
- `TestCredentialMatches_audienceMismatch`: correct issuer+subject, no audience
  overlap → false
- `TestCredentialMatches_match`: exact issuer+subject+audience → true
- `TestCredentialMatches_multipleAudiences`: multiple audiences, one matches → true
- `TestFormatUptime_seconds`: input <60 → `"Xs"` format
- `TestFormatUptime_minutes`: 90 → `"1m30s"`
- `TestFormatUptime_hours`: 3661 → `"1h1m"`
- `TestFormatUptime_exactHour`: 3600 → `"1h0m"`
- `TestStatusIcon_knownStatuses`: "full"→"Full", "stub"→"Stub", "none"→"None",
  case-insensitive variants
- `TestStatusIcon_unknown`: unknown string passthrough
- `TestSetEnvDefaults_setsWhenUnset`: key absent in env → gets set to value
- `TestSetEnvDefaults_doesNotOverride`: key already set → value unchanged
- `TestSetEnvDefaults_multipleKeys`: mix of set/unset keys in same call
- `TestResolveCertFile_configPathExists`: temp file at configPath → returned
- `TestResolveCertFile_configPathAbsent_cwdExists`: no configPath, cwd file
  present → cwd path returned
- `TestResolveCertFile_noFileExists`: nothing on disk, configPath non-empty →
  configPath returned as fallback
- `TestResolveCertFile_emptyConfigPath_noFile`: empty configPath, no file →
  returns empty string

**Verification:** `go test ./cmd/azemu/... -run TestString` etc. pass; coverage
for the listed functions reaches 100%.

---

### U2. `cmd/azemu` HTTP helper tests (`probeHealth` / `waitForHealth`)

**Goal:** Cover `probeHealth` and `waitForHealth` using `httptest.NewServer` so
the package gets HTTP-layer coverage without spawning real processes.

**Dependencies:** U1 (same test file pattern established).

**Files:**

- `cmd/azemu/cmd_http_test.go` (new)

**Approach:**

- Use `httptest.NewServer` with a handler that responds 200 or 500.
- `waitForHealth` with a very short timeout (50ms) exercises the deadline path.
- `waitForHealth` with a server that becomes healthy after a tick exercises the
  poll loop — use a counter inside the handler, not a sleep.

**Test scenarios:**

- `TestProbeHealth_returns200_true`: httptest server returns 200 → true
- `TestProbeHealth_returns500_false`: server returns 500 → false
- `TestProbeHealth_connectionRefused_false`: no server at port → false
- `TestWaitForHealth_alreadyHealthy`: server at URL returns 200 on first poll →
  returns nil error
- `TestWaitForHealth_becomesHealthy`: handler returns 503 twice then 200 →
  returns nil
- `TestWaitForHealth_timeoutExpired`: server always returns 500, 5ms timeout →
  returns error containing "timeout"

**Verification:** `go test ./cmd/azemu/... -run TestProbeHealth -run TestWait`
pass; no external processes spawned.

---

### U3. `cmd/azemu` `ResolveFederatedIdentity` tests

**Goal:** Cover the `federatedIdentityResolver.ResolveFederatedIdentity` method
and its interaction with the in-memory store.

**Dependencies:** U1.

**Files:**

- `cmd/azemu/ficresolver_test.go` (new)

**Approach:**

- Use `store.NewMemoryStore()` from `internal/store` — it is exported.
- Seed the store with a minimal `userAssignedIdentity` resource and one
  `federatedIdentityCredential` child.
- The store key for the identity should follow the production pattern:
  `/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.ManagedIdentity/userAssignedIdentities/id1`
- Test all resolution failure modes independently (wrong clientID, wrong issuer,
  wrong subject, no audience overlap) and the success case.

**Patterns to follow:** `internal/arm/rg_test.go` for store seeding patterns;
`internal/store/memory_test.go` for store construction.

**Test scenarios:**

- `TestResolveFederatedIdentity_found`: matching identity+credential → returns
  `FICMatch{ClientID, PrincipalID, IdentityID}`, ok=true
- `TestResolveFederatedIdentity_clientIDMismatch`: different clientID in props →
  ok=false
- `TestResolveFederatedIdentity_issuerMismatch`: correct clientID, wrong issuer
  on credential → ok=false
- `TestResolveFederatedIdentity_subjectMismatch`: correct issuer, wrong subject
  → ok=false
- `TestResolveFederatedIdentity_audienceMismatch`: no audience overlap → ok=false
- `TestResolveFederatedIdentity_emptyStore`: no identities at all → ok=false
- `TestResolveFederatedIdentity_multipleIdentities`: two identities, only second
  matches → returns second identity's match

**Verification:** `go test ./cmd/azemu/... -run TestResolveFederated` pass.

---

### U4. `internal/ado` gap tests

**Goal:** Raise `endpointBelongsToProject` from 33.3% to 100%; verify
`writeADOJSON`/`writeADOError` error paths.

**Dependencies:** None (existing test files extended).

**Files:**

- `internal/ado/serviceconnection_test.go` (extend)

**Approach:**

- `endpointBelongsToProject` is a pure function — call it directly from tests
  in `package ado_test` (or `package ado` if the file already uses internal
  access — check existing declaration).
- `writeADOJSON`/`writeADOError` error branches (encode failure) require passing
  an unserializable value; use a `chan int` or similar un-marshallable type to
  trigger the marshal error path.

**Test scenarios:**

- `TestEndpointBelongsToProject_noProjectRefs`: endpoint with empty
  `ServiceEndpointProjectReferences` → true for any project
- `TestEndpointBelongsToProject_matchingProject`: refs contains the project →
  true
- `TestEndpointBelongsToProject_differentProject`: refs contains a different
  project ID → false
- `TestEndpointBelongsToProject_multipleRefs`: two refs, one matches → true
- `TestWriteADOJSON_encodeFails`: unserializable body → log error, no panic
  (use `httptest.ResponseRecorder`)
- `TestWriteADOError_encodeFails`: unserializable nested body → log error, no
  panic

**Verification:** `go test ./internal/ado/... -cover` shows `endpointBelongsToProject`
at 100%.

---

### U5. `internal/arm` gap tests

**Goal:** Fill the lowest-coverage functions in `internal/arm`:

- `dns.go::dnsZoneResponse` at 60%
- `federated_identity_credential.go::validateFederatedIdentityCredentialProperties` at 70%
- `keyvault_secret.go::putKeyVaultSecret` at 73.3%

**Dependencies:** None. Uses existing `newTestServer(t)` from
`internal/arm/testutil_test.go`.

**Files:**

- `internal/arm/dns_test.go` (extend)
- `internal/arm/federated_identity_credential_test.go` (extend)
- `internal/arm/keyvault_secret_test.go` (extend)

**Approach:**

- `dnsZoneResponse` — exercised indirectly by GET/PUT DNS zone tests; add
  scenarios that exercise the recordset-count branch (zone with vs. without
  records).
- `validateFederatedIdentityCredentialProperties` — test via `PUT` requests
  with missing/invalid properties; the handler returns 400 for validation
  failures.
- `putKeyVaultSecret` — add a test for the validation error path (empty
  `value` in PUT body).

**Patterns to follow:** Existing tests in `dns_test.go`, `federated_identity_credential_test.go`,
`keyvault_secret_test.go` — use `newTestServer(t)`, `httpPut`, `assertStatus`,
`withAPIVersion`.

**Test scenarios:**

*DNS zone response:*

- `TestDNSZone_responseIncludesRecordSetCount`: after PUT zone + PUT A record,
  GET zone response body includes `numberOfRecordSets ≥ 1`
- `TestDNSZone_emptyZoneResponseShape`: freshly created zone has
  `numberOfRecordSets` and `maxNumberOfRecordSets` fields present

*Federated identity credential validation:*

- `TestFIC_missingIssuer`: PUT with empty `issuer` → 400
- `TestFIC_missingSubject`: PUT with empty `subject` → 400
- `TestFIC_missingAudiences`: PUT with empty `audiences` → 400
- `TestFIC_validProperties`: PUT with all required fields → 201

*Key Vault secret:*

- `TestKVSecret_putEmptyValue`: PUT with `value: ""` → 400 (if validated) or
  check current behaviour and document it; pin it as a regression guard
- `TestKVSecret_putMissingValueField`: PUT body with no `value` key at all →
  document expected status code

**Verification:** `go test ./internal/arm/... -cover` shows all targeted
functions above 80%; overall arm package stays ≥85%.

---

## Scope Boundaries

### Deferred to Follow-Up Work

- E2E/integration tests for `cmd/azemu` subcommands (snapshot, tf, pulumi) —
  these require real Terraform/kubectl/Pulumi binaries and belong in
  `test/integration/`
- Raising `cmd/azemu` coverage above 60% requires process-level mocking
  (exec-replacement pattern) not yet established in this codebase

### Out of Scope

- Changing behaviour of any tested function
- Testing `main()` — standard practice to leave `main` uncovered

---

## Verification

After each unit:

```text
go test ./cmd/azemu/...    # after U1–U3
go test ./internal/ado/... # after U4
go test ./internal/arm/... # after U5
```

Final gate:

```shell
go test ./... -race -cover 2>&1 | grep -E 'coverage:|FAIL'
```

Expected outcome:

- No test failures
- `cmd/azemu` package: ≥60% coverage
- `internal/ado`: all functions ≥80%
- `internal/arm`: all functions ≥80%, package ≥85%
- Total coverage: ≥78% (up from 75.6%)
- `-race` passes clean
