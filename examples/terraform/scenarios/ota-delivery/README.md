# Scenario: server-less OTA delivery (Blob + Key Vault sign + Front Door)

Validates a server-less, static-file over-the-air delivery design end to end
against azemu, with no real Azure subscription. There is no compute on the read
path: a build pipeline signs an update manifest with a Key Vault key and writes
immutable artefacts to Blob Storage, a release pipeline promotes a version by a
server-side blob copy, and Azure Front Door serves the static files to clients.

This scenario stitches the existing `ado-pipeline`, `static-site`, and
`ota-updates` building blocks into one flow and exercises the Front Door read
path through azemu's Front Door content data plane.

## What it provisions

| Resource | Purpose |
|---|---|
| `azurerm_resource_group.main` | Container for everything |
| `azurerm_storage_account.ota` | Immutable artefacts + mutable rollout state (`allow_nested_items_to_be_public = true`) |
| `azurerm_key_vault.signing` | Holds the RSA manifest signing key |
| `azurerm_key_vault_key.manifest` | RSA-2048, `key_opts = ["sign", "verify"]` |
| `azurerm_cdn_frontdoor_profile.ota` | Front Door profile (`Standard_AzureFrontDoor`) |
| `azurerm_cdn_frontdoor_endpoint.ota` | Generated `{name}.azurefd.net` read-path host |
| `azurerm_cdn_frontdoor_origin_group.ota` | Load-balancing settings |
| `azurerm_cdn_frontdoor_origin.ota` | The storage blob origin |
| `azurerm_cdn_frontdoor_route.ota` | Links endpoint to origin group on the default domain |

The blob container is created by the publish step, not Terraform: azemu does
not mirror ARM containers into the Azurite data plane. The `azemuotadsa` account
ships pre-registered in `docker-compose.yml` `AZURITE_ACCOUNTS`.

## Blob path layout

```text
/ota/{runtimeVersion}/{channel}/{platform}/
    v{n}/
        bundle-<hash>.js       # immutable
        asset-<hash>.bin       # immutable
        update.multipart       # immutable, pre-signed multipart/mixed
    manifest.json              # live, multipart/mixed, short TTL
    rollout.json               # live release state, short TTL
```

Hashed assets carry `Cache-Control: public, max-age=31536000, immutable`; the
live `manifest.json` and `rollout.json` carry a short TTL so releases propagate
quickly. A promotion is a server-side copy of `v{n}/update.multipart` to the
live `manifest.json` path; the signature is over the manifest bytes and travels
in the multipart part header, so the copy never invalidates it.

## The Front Door read path

azemu's Front Door content data plane serves the endpoint host
`{name}.azurefd.net` (multiplexed on the ARM port `:4566`): it resolves the
endpoint, walks route to origin group to origin to find the Blob origin
(`{account}.blob.core.windows.net`), and reverse-proxies to Azurite path-style,
passing the origin's `Content-Type` and `Cache-Control` through unchanged. A
client therefore fetches the signed manifest and assets from the Front Door
host, exactly as in production.

Because `{name}.azurefd.net` is not a real DNS name locally, the client
resolves it to `127.0.0.1` (the `fixturegen verify` tool and the `curl --resolve`
examples below do this) and trusts the azemu cert (covered by the
`*.azurefd.net` SAN).

## Run the full loop (local)

```bash
make ota-delivery          # from the repo root
```

That brings up azemu + Azurite, provisions the ARM estate, publishes a signed
update, promotes it to 100%, asserts the Front Door read path (multipart Content-Type,
cache TTLs, and the manifest signature against the exported public key), then
tears everything down. It needs Docker (for the Azurite data plane) and Go (for
the `fixturegen` tool).

## CI (ARM-half smoke)

CI runs only the ARM half via `terraform test` (`main.tftest.hcl`): it provisions
the estate and asserts the control-plane outputs. The publish + serve-assert
loop is local-only because it needs the running Azurite data plane.

```bash
docker compose up -d --build              # from the repo root
export SSL_CERT_FILE=$PWD/.azemu/cert-bundle.pem
cd examples/terraform/scenarios/ota-delivery
terraform init && terraform test
```

## Manual read-path checks

```bash
FQDN=$(terraform output -raw cdn_endpoint_fqdn)
CERT=$PWD/../../../../.azemu/cert-bundle.pem

# manifest: multipart/mixed, short TTL
curl -isS --resolve "$FQDN:4566:127.0.0.1" --cacert "$CERT" \
  "https://$FQDN:4566/ota/1.0.0/PRODUCTION/android/manifest.json"

# an asset: immutable long TTL
curl -isS --resolve "$FQDN:4566:127.0.0.1" --cacert "$CERT" \
  "https://$FQDN:4566/ota/1.0.0/PRODUCTION/android/v1/asset-<hash>.bin"
```

If your shell sends these through an HTTP proxy, set
`NO_PROXY=$FQDN,127.0.0.1,localhost` first.

## Boundary

The real manifest, rollback-directive, and multipart generators used by a
production pipeline are out of scope. The `fixturegen` tool here is azemu's own
minimal, independent generator written from the public Expo Updates Protocol v1
shape; its only purpose is to produce a believable signed artefact so the
read-path assertions have something to fetch.
