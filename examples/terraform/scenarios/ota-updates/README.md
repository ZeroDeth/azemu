# Scenario: OTA update pipeline

Infrastructure for an over-the-air update pipeline in the expo-updates
self-hosted static-file model: signed update bundles live in Blob Storage,
the manifest signing key lives in Key Vault, and there is no server in the
runtime path. The app downloads `manifest.json` and bundle assets as static
files over HTTP and verifies the manifest signature with a public key baked
into the build.

## Resources

| Resource | Purpose |
|---|---|
| `azurerm_resource_group.main` | Container for everything |
| `azurerm_storage_account.updates` | Update bundles and manifests (`allow_nested_items_to_be_public = true`) |
| `azurerm_key_vault.signing` | Holds the manifest signing key |
| `azurerm_key_vault_key.manifest` | RSA-2048, `key_opts = ["sign", "verify"]` |

The blob container is created by the publish script, not Terraform: azemu
does not mirror ARM containers into the Azurite data plane. The script
issues one Create Container call against Azurite with
`x-ms-blob-public-access: blob` before the first upload. See
`docs/SETUP.md` "Storage account names and AZURITE_ACCOUNTS"; the
`azemuotasa` account used here ships pre-registered in
`docker-compose.yml`.

## Run

```bash
docker compose up -d --build              # from the repo root
export SSL_CERT_FILE=$PWD/.azemu/cert-bundle.pem
cd examples/terraform/scenarios/ota-updates
terraform init && terraform test
```

## Publish-time sign call

Terraform outputs `manifest_key_id` (the versioned kid). The publish script
signs the SHA-256 digest of the manifest with one REST call (portable
base64url pipeline, no GNU coreutils needed):

```bash
KID=$(terraform output -raw manifest_key_id)
DIGEST=$(openssl dgst -sha256 -binary manifest.json \
  | openssl base64 -A | tr '+/' '-_' | tr -d '=')
curl -sk -X POST "$KID/sign?api-version=7.4" \
  -H 'Content-Type: application/json' \
  -d "{\"alg\":\"RS256\",\"value\":\"$DIGEST\"}"
```

The response `value` is the base64url RS256 signature. It verifies against
the `manifest_key_public_pem` output:

```bash
terraform output -raw manifest_key_public_pem > pubkey.pem
openssl dgst -sha256 -verify pubkey.pem -signature manifest.sig manifest.json
```
