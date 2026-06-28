# Design note 1: Delegate Storage data plane to Azurite

<div class="designnote-meta">
<span class="designnote-meta-item"><span class="designnote-status designnote-status--implemented">Implemented</span></span>
<span class="designnote-meta-item"><strong>Date</strong> 2026-04-21</span>
<a href="https://github.com/ZeroDeth/azemu/blob/main/docs/design-notes/0001-delegate-storage-data-plane-to-azurite.md" class="designnote-github-link">Full text on GitHub →</a>
</div>

<div class="designnote-decision" markdown>

<span class="designnote-decision-label">▸ DECISION</span>

**azemu implements the Storage management plane (ARM) and delegates the
Storage data plane to Azurite, shipped as a sidecar in `docker-compose.yml`.**

- azemu serves `Microsoft.Storage/storageAccounts` CRUD, `listKeys`, and ARM
  sub-resources (blob containers, file shares) the azurerm provider uses.
- `listKeys` returns Azurite's well-known account keys so SDK clients succeed
  against the sidecar.
- `primaryEndpoints` in ARM responses point at the Azurite sidecar using
  path-style URLs; no `/etc/hosts` edit required.
- `docker-compose.yml` adds an `azurite` service. One env var controls the
  endpoint: `AZEMU_AZURITE_ENDPOINT` (default `http://azurite:10000`).

</div>

## Consequences

### Positive

- Phase 7 scope shrinks. Only the Storage management plane and `listKeys`
  need authoring.
- Future data-plane features (versioning, lifecycle, SAS) arrive via Azurite
  without azemu work.
- `ghcr.io/zerodeth/azemu` stays a single-purpose image. Storage is opt-in.
- The roadmap positioning table becomes an honest statement.

### Negative

- Two containers for users who exercise Storage. Mitigated: both start under
  one `docker compose up` and the Azurite image is under 200 MB.
- Error-message parity is bounded by Azurite's parity with real Azure.
  Documented in the [parity matrix](../../concepts/parity-matrix.md).
- `primaryEndpoints` rewriting is a new responsibility for the ARM handlers.

### Neutral

- Contributors working on Storage need Azurite locally. The flox environment
  picks it up as a dev dependency when Phase 7 opens.
- ADO and AKS work in v0.3 is unaffected.
