# Design note 3: Add Azure Cache for Redis

<div class="designnote-meta">
<span class="designnote-meta-item"><span class="designnote-status designnote-status--implemented">Implemented</span></span>
<span class="designnote-meta-item"><strong>Date</strong> 2026-04-28</span>
<a href="https://github.com/ZeroDeth/azemu/blob/main/docs/design-notes/0003-add-azure-cache-for-redis.md" class="designnote-github-link">Full text on GitHub →</a>
</div>

<div class="designnote-decision" markdown>

<span class="designnote-decision-label">▸ DECISION</span>

**Add `azurerm_redis_cache` (`Microsoft.Cache/Redis`) to the v0.2 resource
roster. azemu serves the ARM management plane; the data plane is delegated to
a standard `redis` sidecar in `docker-compose.yml`, mirroring the Azurite
pattern from design note 1.**

- azemu serves `Microsoft.Cache/Redis` CRUD, `POST .../listKeys`, capacity and
  family validation, and the SKU shape the azurerm provider expects.
- `listKeys` returns deterministic dev keys; the sidecar is configured with
  `--requirepass` matching the primary key so auth round-trips work in tests.
- `hostName` in ARM responses points at the redis sidecar
  (`azemu-redis` inside Docker, `localhost:6379` for host callers).
- `docker-compose.yml` adds an optional `redis` service via compose profiles;
  default users see no change.

</div>

## Consequences

### Positive

- The multi-replica scenario in design note 2 becomes fully expressible without
  extra infrastructure beyond what compose already orchestrates.
- The pattern generalises: future "real backend" data-plane work follows the
  same recipe.
- `azurerm_redis_cache` round-trips a real `terraform apply` against
  unmodified `azurerm`, adding one more "Full" entry to PARITY.md.

### Negative

- One more optional container in compose. Mitigated by compose profiles so
  default users see no change.
- `listKeys` dev keys must match the sidecar's `--requirepass`. Documented
  in [setup](../../reference/setup.md).
- Premium-tier shapes (clustering, geo-replication, persistence) are tracked
  as follow-ups. The v0.2 row reads "Standard tier only."

### Neutral

- Redis client library choice is left to the contributor (go-redis, ioredis, redis-py).
- Existing v0.1 and v0.2 functionality is unchanged.

## Open questions

- **Premium tier features.** Initial scope is Standard-only; Premium-tier
  clustering is a tracked follow-up.
- **TLS port 6380.** The first implementation may default to non-TLS for
  simplicity. Stunnel opt-in documented as a follow-up.
