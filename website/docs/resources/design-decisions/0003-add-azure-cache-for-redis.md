# Design Decision 0003: Add Azure Cache for Redis

- Status: Implemented
- Date: 2026-04-28
- Implemented: 2026-04-28
- Deciders: @ZeroDeth
- Supersedes: none

## Context

Production AKS workloads commonly require a managed Redis instance for
multi-replica cache coherence, session storage, rate-limiting state, or
event coordination. A representative example is a self-hosted
[expo-open-ota](https://github.com/expo/expo-open-ota) deployment: three
replicas behind a load balancer with `CACHE_MODE=redis`, backed by an
Azure Cache for Redis instance so manifest lookups and signed-bundle
metadata stay coherent across pods.

azemu's current resource roster has no Redis story. ROADMAP.md does not
list `azurerm_redis_cache` in v0.2 or v0.3, and "Beyond v0.3" only
mentions Cosmos DB and Event Grid. Contributors who want to round-trip
a Terraform config that includes Redis hit a wall.

Two surfaces are involved:

1. **ARM management plane** (`Microsoft.Cache/Redis`): cluster CRUD,
   `listKeys`, SKU, capacity, family, redis configuration. This is the
   surface `azurerm_redis_cache` Terraform resource exercises.
2. **Redis data plane** (RESP protocol on `:6379` or `:6380` for TLS):
   `GET`, `SET`, `EXPIRE`, pub/sub, the full Redis command surface.

Reimplementing the Redis data plane is unnecessary: the upstream `redis`
container image is the canonical implementation, ships in single-digit
megabytes, and is already what most production Redis deployments run
under the hood.

## Decision

**Add `azurerm_redis_cache` (Microsoft.Cache/Redis) to the v0.2
resource roster. azemu serves the ARM management plane; the data plane
is delegated to a standard `redis` sidecar in `docker-compose.yml`,
mirroring the Azurite pattern from ADR 0001.**

In concrete terms:

- azemu serves `Microsoft.Cache/Redis` CRUD, `POST .../listKeys`,
  capacity and family validation, and the SKU shape the azurerm
  provider expects. SSL/non-SSL ports are reported in the response.
- `listKeys` returns deterministic dev keys (e.g., the literal
  `azemu-dev-primary-key`/`...-secondary-key`) so SDK clients
  authenticated by these keys succeed against the sidecar. Real Redis
  does not authenticate by these keys; the sidecar is configured with
  `--requirepass` matching the primary so the round-trip works for
  tests that exercise auth.
- The `hostName` in ARM responses points at the redis sidecar
  (`azemu-redis` inside the docker network, `localhost:6379` for host
  callers), set via a single env var (`AZEMU_REDIS_ENDPOINT`, default
  `redis://azemu-redis:6379`).
- `docker-compose.yml` adds an optional `redis` service. Profiles or
  `depends_on` keep it dormant for users who do not exercise Redis.
- Outside Docker (flox workflow), Redis is an optional process; ARM
  management-plane tests do not require it. Only end-to-end scenarios
  that touch the Redis data plane pin the sidecar as a test
  dependency.

## Rationale

1. **Same pattern as ADR 0001.** Storage delegates to Azurite, Redis
   delegates to upstream `redis`. azemu's job is the ARM surface; the
   data planes belong to their canonical emulators.

2. **Multi-replica deployment scenarios need it.** The expo-open-ota
   example above is one instance; any multi-replica AKS workload with
   shared state lands in the same shape. Without Redis, the
   `aks-workload/` scenario from ADR 0002 cannot fully express what
   production deployments do.

3. **Negligible footprint.** `redis:7-alpine` is under 30 MB. Optional
   in compose so users who do not need it pay zero startup cost.

4. **Honest fidelity.** The ARM stub round-trips a real
   `azurerm_redis_cache` resource against unmodified `azurerm`, and the
   data plane is the real upstream Redis the workload would talk to in
   production. No hand-rolled RESP, no fake `GET`/`SET` semantics.

## Consequences

### Positive

- The multi-replica scenario in ADR 0002 becomes fully expressible
  without extra infrastructure beyond what compose already orchestrates.
- The pattern generalises: future "real backend" data-plane work
  (Cosmos DB Mongo API, PostgreSQL flexible server) follows the same
  recipe.
- `azurerm_redis_cache` round-trips a real `terraform apply` against
  unmodified `azurerm`, adding one more "Full" entry to PARITY.md.

### Negative

- One more optional container in compose. Mitigated by docker-compose
  profiles so default users see no change.
- `listKeys` returns dev keys that the sidecar's `--requirepass` must
  match. Documented in `docs/SETUP.md`. A future `regenerateKey`
  endpoint requires re-issuing the sidecar password, which is only
  meaningful for scenarios that test rotation.
- Premium-tier shapes (clustering, geo-replication, persistence) are
  out of scope for the initial implementation and tracked as
  follow-ups. The v0.2 row reads "Standard tier only" until a follow-up
  closes that gap.

### Neutral

- The redis client library used inside scenarios is the contributor's
  choice (go-redis, ioredis, redis-py); azemu does not constrain it.
- Existing v0.1 / v0.2 functionality is unchanged.

## Alternatives considered

1. **Hand-roll a Redis-compatible server in Go inside azemu.**
   Rejected. RESP is small, but full command coverage is large, drifts
   per Redis version, and competes with `redis-server` itself. Same
   reasoning as ADR 0001 alternative 1.

2. **Skip Redis entirely; tell users to wire their own outside
   `docker-compose.yml`.** Rejected. The multi-replica scenario in ADR
   0002 needs a wired-in Redis to be reproducible, and "wire your own"
   moves friction from azemu to every scenario author.

3. **Use a Redis-compatible alternative (KeyDB, Dragonfly).**
   Rejected for the default. Both work, but the Azure resource is
   `Microsoft.Cache/Redis`, the production target is the real Redis
   protocol on a specific version line, and adopting an alternative
   adds a dimension of difference between azemu and real Azure that
   does not pay for itself. Contributors can swap the image in their
   own compose override if they prefer.

4. **Defer Redis to v0.4.** Rejected. The multi-replica scenario class
   is in active demand, and deferring leaves the "azemu cannot
   exercise this real shape" gap open for a full milestone.

## Implementation (Phase 7.7, proposed)

The following is the planned scope when `azurerm_redis_cache` lands:

- `internal/arm/redis_cache.go`: handlers for PUT / GET / DELETE / HEAD
  / LIST under
  `subscriptions/.../resourceGroups/.../providers/Microsoft.Cache/Redis/{name}`
  plus `POST .../listKeys`.
- `internal/arm/redis_cache_test.go`: unit tests covering CRUD, SKU
  validation, `listKeys` shape, parent existence (RG must exist),
  cascade delete on RG removal.
- `pkg/config/config.go`: `AZEMU_REDIS_ENDPOINT` env var (default
  `redis://azemu-redis:6379`).
- `internal/metadata/service.go` / metadata suffix: confirm the
  `redisCache` suffix entry matches go-azure-sdk expectations
  (`redis.cache.windows.net`); add a regression test alongside the
  existing `TestMetadata_CanonicalSuffixNames`.
- `docker-compose.yml`: optional `redis` service using
  `redis:7-alpine`, port 6379, `--requirepass azemu-dev-primary-key`,
  a healthcheck. Behind a `redis` compose profile so users opt in.
- `examples/terraform/scenarios/aks-workload/`: integrates the Redis
  cache into the multi-replica scenario from ADR 0002.
- `docs/SETUP.md`: Redis section added; `AZEMU_REDIS_ENDPOINT`
  documented in the env-var table.
- `docs/PARITY.md`: new row for `azurerm_redis_cache` with data plane
  column reading "Delegated to upstream redis sidecar".

## Open questions

- **Premium tier features (clustering, persistence, geo-replication).**
  Initial scope is Standard-only. Premium-tier shapes (especially
  cluster mode) are tracked as follow-ups and called out in
  `docs/PARITY.md`.
- **TLS port 6380.** Real Azure Cache for Redis exposes a TLS endpoint
  on 6380. The sidecar can stunnel-wrap the plain port, but the first
  implementation may default to non-TLS for simplicity. Decide during
  Phase 7.7 build-out.

## References

- ROADMAP.md v0.2 resource roster (this ADR adds a row).
- ADR 0001 (delegate Storage data plane to Azurite): the precedent for
  upstream-emulator delegation.
- ADR 0002 (azemu + kind hybrid for AKS workload deployments): the
  consumer scenario for this resource.
- [Redis](https://redis.io/), [Azure Cache for
  Redis](https://learn.microsoft.com/en-us/azure/azure-cache-for-redis/).
