# Design Decision 0001: Delegate Storage data plane to Azurite

- Status: Implemented
- Date: 2026-04-21
- Implemented: 2026-04-22
- Deciders: @ZeroDeth
- Supersedes: none

## Context

Phase 7 of the v0.2 milestone ships
`azurerm_storage_account` and `azurerm_storage_container`. Making those
Terraform resources round-trip end-to-end requires two distinct API
surfaces:

1. The ARM management plane
   (`https://management.azure.com/.../Microsoft.Storage/storageAccounts/...`)
   for account and container CRUD.
2. The Storage data plane
   (`https://<account>.blob.core.windows.net/...`) for any operation the
   azurerm provider routes through the Azure Storage SDK, plus all
   client-side workflows that read or write blobs, queues, or tables.

The data plane is a large surface: shared-key HMAC signing, SAS token
issuance and validation, block blob chunking and commit semantics, CORS
rules, lifecycle management, static-website hosting, soft delete,
versioning, and per-service DNS suffixes for blob, queue, table, file,
dfs, and web.

Microsoft ships
[Azurite](https://learn.microsoft.com/en-us/azure/storage/common/storage-use-azurite)
as the official open-source emulator for the Storage data plane. Azurite
supports Blob, Queue, and Table services, supersedes the legacy Azure
Storage Emulator, and is actively maintained against current Storage API
versions.

The question is whether azemu should implement the Storage data plane
itself, or delegate it to Azurite while owning the ARM management plane.

## Decision

**azemu implements the Storage management plane (ARM) and delegates the
Storage data plane to Azurite, shipped as a sidecar in `docker-compose.yml`.**

In concrete terms:

- azemu serves `Microsoft.Storage/storageAccounts` CRUD, `listKeys`,
  `regenerateKey`, and the ARM sub-resources
  (`.../blobServices/containers`,
  `.../fileServices/shares`) that the azurerm provider uses for
  management-plane calls.
- `listKeys` returns Azurite's well-known account keys so SDK clients
  authenticated by those keys succeed against the sidecar.
- The `primaryEndpoints` block in ARM responses points at the Azurite
  sidecar, using IP-style URLs
  (`http://localhost:10000/<account>`) so no `/etc/hosts` edit is
  required for the first-time user.
- `docker-compose.yml` adds an `azurite` service on the project network.
  The azemu binary learns the Azurite service address from a single env
  var (`AZEMU_AZURITE_ENDPOINT`, default `http://azurite:10000`).
- When azemu runs outside Docker (flox workflow), Azurite is an optional
  process; the ARM management-plane tests do not require it. Only the
  end-to-end scenarios that touch blob data pin Azurite as a test
  dependency.

## Rationale

1. **Do not reimplement what upstream already ships.** azemu's founding
   move is intercepting the `metadata_host` field of the azurerm provider
   to avoid forking it. Delegating the Storage data plane to Azurite is
   the same move one layer down: use the official artifact, keep our
   surface narrow. Microsoft tracks Storage API versions; we inherit that
   cadence for free.

2. **Scope containment protects fidelity.** The v0.1 PARITY matrix earns
   its "Full" marks because the three shipped resources each round-trip a
   real `terraform apply`. Hand-rolling HMAC, SAS, block commit, and six
   DNS suffixes would eat the Phase 7 budget and drop fidelity on
   resources that do not even belong to the Storage family.

3. **Roadmap non-goals already point this way.** The current roadmap
   states: "Storage data-plane goes exactly deep enough that the azurerm
   provider's container creation and blob-metadata writes round-trip."
   The positioning table also credits Azurite as "data-plane fidelity for
   one service" and hints at deferral. This decision promotes that hint
   to a commitment.

4. **User experience stays one command.** `docker compose up -d` now
   brings up two containers instead of one, behind the same
   `metadata_host` intercept. No hosts-file edit, no new TLS trust step,
   no new ports in the default setup beyond Azurite's own 10000-10002.

5. **Testing cost is lower, not higher.** Phase 7 tests for containers
   and blobs become integration tests that assert on real Azurite
   responses rather than on hand-rolled fakes. If Azurite drifts from
   Azure on an error code, we inherit the drift from Microsoft, which is
   the same deal every other user of Azurite accepts.

## Consequences

### Positive

- Phase 7 shrinks. The Storage Account management-plane CRUD, `listKeys`,
  and the minimum ARM sub-resources are all that azemu has to author.
- Future data-plane features (versioning, lifecycle, SAS) arrive without
  azemu work as Azurite releases them.
- `ghcr.io/zerodeth/azemu` stays a single-purpose image. Users who do not
  need Storage skip the Azurite container.
- The roadmap positioning table becomes an honest statement instead of an
  implicit deferral.

### Negative

- Two containers for users who exercise Storage. Mitigated: both start
  under one `docker compose up` and the Azurite image is under 200 MB.
- Error-message parity is bounded by Azurite's parity with real Azure.
  Documented in the Storage section of the [parity matrix](../../concepts/parity-matrix.md)
  when Phase 7 lands.
- `primaryEndpoints` rewriting is a new responsibility for the ARM
  handlers. Tested alongside `listKeys` in Phase 7.
- The azemu binary gains one configuration knob
  (`AZEMU_AZURITE_ENDPOINT`). Documented in the [setup guide](../../reference/setup.md)
  with a sensible default that matches the shipped `docker-compose.yml`.

### Neutral

- Contributors working on Storage need Azurite locally. The flox
  environment picks it up as a dev dependency when Phase 7 opens; no
  action required before that.
- ADO and AKS work in v0.3 is unaffected by this decision.

## Alternatives considered

1. **Reimplement the Storage data plane in Go inside azemu.** Rejected.
   The HMAC, SAS, and block-commit surfaces are large, and Microsoft
   already maintains Azurite for this job. Fidelity would lag, not lead.

2. **Delegate to Azurite but run it embedded via a Node.js sidecar inside
   the azemu container.** Rejected. Mixes runtimes inside one image,
   complicates the Dockerfile, and loses the clean "optional second
   container" on/off switch that separate services give us.

3. **Point `primaryEndpoints` straight at Azurite and skip the
   management-plane `listKeys` + rewrite.** Rejected. The azurerm
   provider calls `listKeys` on the ARM endpoint before it ever hits the
   data plane; the call must succeed with credentials Azurite accepts, or
   every container resource fails at plan time.

4. **Production-style URLs via `/etc/hosts` entries
   (`<account>.blob.localhost`).** Rejected as the default. Azurite
   supports it, but editing `/etc/hosts` is a setup tax we already avoid
   elsewhere. IP-style URLs stay the default; the production-style
   variant can be documented as an opt-in later if a user needs SDK code
   that rejects path-style account names.

## Implementation (Phase 7, completed 2026-04-22)

The following was built and shipped on branch `feat/phase7-storage-account`:

- `pkg/config/config.go`: `AZEMU_AZURITE_ENDPOINT` env var (default
  `http://azurite:10000`). azemu derives queue (`:10001`) and table
  (`:10002`) base URLs from this single knob by replacing the port.
- `internal/arm/router.go`: `Router.azuriteEndpoint` field; `NewRouter`
  signature updated to accept it; `POST .../listkeys` route added.
- `internal/arm/storage_account.go`: `storagePrimaryEndpoints` rewritten
  to produce path-style Azurite URLs for blob/queue/table/file/web/dfs.
  `listStorageAccountKeys` handler returns Azurite's well-known dev key.
- `docker-compose.yml`: `azurite` service using
  `mcr.microsoft.com/azure-storage/azurite` with ports 10000-10002,
  a named volume (`azurite-data`), a healthcheck, and `depends_on`
  (condition: service\_healthy) so azemu only starts after Azurite is ready.
- Setup guide: Storage and Azurite section added; `AZEMU_AZURITE_ENDPOINT`
  documented in the env-var table.
- Parity matrix: Storage Accounts row updated; data plane column now
  reads "Delegated to Azurite".

**Note on URL style:** the decision described "IP-style URLs
(`http://localhost:10000/<account>`)" as the default. What shipped uses the
same path-style layout but derives the host from `AZEMU_AZURITE_ENDPOINT`,
so the hostname inside Docker is `azurite` rather than `localhost`. This
is strictly compatible with the decision; the docker-compose hostname is
what users actually see inside Docker networks.

**Deferred items** (not blocked by this decision but not yet implemented):

- `regenerateKey` action endpoint.
- `.../fileServices/shares` ARM sub-resources.
- `examples/terraform/scenarios/static-site/` end-to-end scenario.

## References

- [Azurite emulator](https://learn.microsoft.com/en-us/azure/storage/common/storage-use-azurite)
- [Azurite GitHub](https://github.com/Azure/Azurite)
- [Roadmap](../../community/roadmap.md) v0.2 resource roster and positioning table
- [Architecture](../../concepts/architecture.md) request flow (metadata intercept)
