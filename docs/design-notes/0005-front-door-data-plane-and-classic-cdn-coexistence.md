# Design note 5: Front Door data plane and classic CDN coexistence

- Status: Implemented
- Date: 2026-06-29

## Context

azemu's production target at Tesco serves over-the-air (OTA) update
bundles through Azure Front Door. The OTA read path (signed manifest plus
immutable assets in Blob Storage, fronted by a CDN) is the shape design
note 1 and the `ota-delivery` scenario were built to mirror locally.

Until now azemu emulated only classic Azure CDN: the
`azurerm_cdn_profile` / `azurerm_cdn_endpoint` control plane and a
`*.azureedge.net` content data plane (design note for the data plane
landed in v0.3.0). The azurerm provider removed classic CDN at v4.35.0
(the resources fail a wall-clock deprecation check client-side before any
request reaches azemu), so the `static-site` and `ota-delivery` scenarios
were pinned to `>= 4.0, < 4.35`. That pin meant azemu could not validate
the resource graph the production system actually ships, which is Front
Door, not classic CDN. This note records how Front Door was added without
removing classic CDN.

Front Door (Standard/Premium) spreads what classic CDN packs into one
endpoint across a resource graph:

```text
afdEndpoint --> route --> originGroup --> origin (Blob host)
```

The endpoint advertises a generated `*.azurefd.net` host; a route links
that endpoint to an origin group; the origin group holds one or more
origins; each origin carries the backing Blob host. A client fetches from
the endpoint host, and the edge walks the graph to the origin.

## Decision

**Add Front Door as a second CDN surface that coexists with classic CDN.
Both control planes and both content data planes run side by side; the
host-mux on the ARM port dispatches by host suffix. Migrate the
`static-site` and `ota-delivery` scenarios to Front Door and lift their
`< 4.35` pin; leave classic CDN in place for users still pinned below
4.35.**

Concretely:

- **Shared profile type.** `azurerm_cdn_frontdoor_profile` is the same ARM
  type as classic CDN, `Microsoft.Cdn/profiles`. The existing profile
  handler already accepts any SKU, so it serves Front Door profiles
  (`Standard_AzureFrontDoor`, `Premium_AzureFrontDoor`) unchanged. Only
  the four Front Door child types are new: `afdEndpoints`, `originGroups`,
  `origins`, `routes`, all under `internal/arm/cdn_frontdoor.go`.
- **SKU is a no-op.** Standard versus Premium changes nothing in
  emulation. The SKU is stored and echoed back so the provider round-trips
  it; azemu does not gate any behaviour on it.
- **Deterministic endpoint host.** On `afdEndpoint` create, azemu writes a
  read-only `hostName` of `{name}.azurefd.net` into the response. Real
  Azure mints `{name}-{hash}.z01.azurefd.net`; azemu uses the stable form
  so the value the provider reads (and the scenario references via
  `azurerm_cdn_frontdoor_endpoint.X.host_name`) is exactly the host the
  data plane resolves. No hash to reconcile.
- **Host-mux resolution.** A request to `{endpoint}.azurefd.net` on the
  ARM port is matched by suffix and handed to the Front Door content data
  plane (`internal/arm/cdn_frontdoor_dataplane.go`), which walks
  endpoint -> route -> originGroup -> origin, reads the origin's Blob host,
  and reverse-proxies path-style to Azurite. `*.azureedge.net` still goes
  to the classic CDN proxy; everything else to the ARM control plane. The
  two data planes share one blob-proxy helper, so the origin's
  `Content-Type` and `Cache-Control` reach the client unchanged for both
  (the OTA manifest keeps its multipart boundary and short TTL).
- **Minimal resource depth.** Only the five resources the scenarios need
  are implemented. `cdn_frontdoor_custom_domain`, `rule_set`, `rule`,
  `security_policy`, and `firewall_policy` are out of scope. The route
  sets `link_to_default_domain = true`, which serves on the default
  `*.azurefd.net` host with no custom domain, so none of those are
  required.

## Rationale

1. **Fidelity to the production target.** The OTA system ships Front Door.
   With classic CDN only, azemu could not exercise the resource graph
   under test. Front Door closes that gap.
2. **No breakage for pinned users.** Removing classic CDN would break any
   user still on azurerm `< 4.35`. Both surfaces share the profile type
   and the blob-proxy core, so coexistence costs little and keeps the
   old path green.
3. **Deterministic host avoids a reconcile step.** A hashed host would
   force azemu to store the generated host and the provider to read it
   back before the data plane could resolve it. The stable
   `{name}.azurefd.net` form makes control plane and data plane agree by
   construction, the same approach the classic `*.azureedge.net` data
   plane already uses.
4. **Synchronous writes are enough.** The provider pins the Front Door
   child types to the track1 `cdn/2021-06-01` SDK, whose long-running
   operation future is satisfied by a terminal `200`/`201` with no async
   header. The profile uses `2024-02-01`, whose poller treats a `200` with
   `provisioningState: "Succeeded"` as immediate success. azemu answers
   both synchronously; DELETE reuses the existing async-operation path from
   design note material in `operations.go` (the M7 lesson).

## Consequences

### Positive

- `static-site` and `ota-delivery` run at azurerm `>= 4.35` against the
  real Front Door resource graph, so the pin is lifted for both.
- The OTA read path is validated end to end through Front Door, matching
  production.
- Two more "Full" entries on the parity matrix (the Front Door child
  graph plus the `*.azurefd.net` data plane).

### Negative

- Two CDN surfaces to maintain. Mitigated by the shared profile handler
  and the shared blob-proxy helper, so the duplication is small.
- Front Door depth is intentionally shallow: no custom domains, rule sets,
  WAF, or caching-rule overrides. Tracked as a follow-up; the route's
  `link_to_default_domain = true` keeps the minimal config valid.

### Neutral

- SKU (Standard versus Premium) is stored but inert. A future feature that
  depends on the tier (private link, WAF) would change that.
- Origin selection picks the lowest `priority` then highest `weight`. In
  the single-origin scenarios this is the only origin; the ordering
  matters only if a group ever lists several.

## Alternatives considered

1. **Replace classic CDN with Front Door.** Rejected. It would break users
   pinned below azurerm 4.35 for no benefit, since the surfaces coexist
   cheaply.
2. **Match Azure's hashed `{name}-{hash}.z01.azurefd.net` host.**
   Rejected. The hash adds a reconcile step between control and data plane
   with no fidelity gain for a local emulator; the data plane controls
   both ends of the host already.
3. **Implement the full Front Door surface (custom domains, rule sets,
   WAF) now.** Rejected as premature. No scenario needs it; the five core
   resources plus `link_to_default_domain` cover the OTA and static-site
   read paths. Depth is added when a scenario demands it.

## References

- design note 1 (delegate Storage data plane to Azurite): the origin the
  Front Door data plane proxies to.
- design note 3 (Azure Cache for Redis): the same control-plane-emulated,
  data-plane-delegated pattern.
- TODO.md M7: the async-DELETE operation-result lesson reused here.
- [Azure Front Door
  documentation](https://learn.microsoft.com/en-us/azure/frontdoor/).
