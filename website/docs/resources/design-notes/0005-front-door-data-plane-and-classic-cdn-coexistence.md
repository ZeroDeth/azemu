# Design note 5: Front Door data plane and classic CDN coexistence

<div class="designnote-meta">
<span class="designnote-meta-item"><span class="designnote-status designnote-status--implemented">Implemented</span></span>
<span class="designnote-meta-item"><strong>Date</strong> 2026-06-29</span>
<a href="https://github.com/ZeroDeth/azemu/blob/main/docs/design-notes/0005-front-door-data-plane-and-classic-cdn-coexistence.md" class="designnote-github-link">Full text on GitHub →</a>
</div>

<div class="designnote-decision" markdown>

<span class="designnote-decision-label">▸ DECISION</span>

**Add Front Door as a second CDN surface that coexists with classic CDN. Both
control planes and both content data planes run side by side; the host-mux on
the ARM port dispatches by host suffix. Migrate the `static-site` and
`ota-delivery` scenarios to Front Door and lift their `< 4.35` pin; leave
classic CDN in place: it is not removed from the provider, only blocked from
creation at 4.35 by a wall-clock check, with updates supported until
2027-09-30.**

- The production OTA read path ships Azure Front Door. With classic CDN only,
  azemu could not exercise the resource graph under test.
- All five Front Door resources update via PATCH, not PUT. The first version of
  this note analysed only the create path, so PATCH was never registered and
  the resources provisioned once then failed on every later `terraform apply`.
  They now share the emulator's PATCH merge.
- `azurerm_cdn_frontdoor_profile` is the same ARM type as classic CDN
  (`Microsoft.Cdn/profiles`), so the existing profile handler serves it
  unchanged. Only the four child types are new: afdEndpoints, originGroups,
  origins, routes. SKU (Standard versus Premium) is a no-op.
- An endpoint advertises a deterministic `{name}.azurefd.net` host, so the
  control plane and the `*.azurefd.net` data plane agree by construction. The
  data plane walks endpoint to route to origin group to origin and proxies to
  the Blob origin, the same blob-proxy core the classic `*.azureedge.net` plane
  uses.

</div>

## Consequences

### Positive

- `static-site` and `ota-delivery` run at azurerm `>= 4.35` against the real
  Front Door resource graph, so the pin is lifted.
- The OTA read path is validated end to end through Front Door, matching
  production.
- Two more "Full" entries on the parity matrix: the Front Door child graph and
  the `*.azurefd.net` data plane, both covering create and update.

### Trade-offs

- Two CDN surfaces to maintain, kept small by the shared profile handler and
  shared blob-proxy helper.
- Front Door depth is intentionally shallow: no custom domains, rule sets, WAF,
  or caching-rule overrides. The route's `link_to_default_domain = true` keeps
  the minimal config valid; deeper features are a follow-up.

### Lesson

- Analysing only the create path is what produced the update bug. `terraform
  test` applies and then destroys, so it cannot catch a missing update verb;
  nothing failed until a second apply changed an attribute. Any resource whose
  provider updates by PATCH will repeat this until a scenario applies twice.

### Neutral

- SKU is stored but inert. A future feature that depends on the tier (private
  link, WAF) would change that.
- Origin selection picks the lowest `priority` then highest `weight`. In the
  single-origin scenarios this is the only origin.
- Endpoint names resolve globally, because the data plane has only the Host
  header and the host derives from the name alone. Two endpoints sharing a name
  in different profiles collide by construction; the resolver sorts by ID so the
  choice is at least stable.
