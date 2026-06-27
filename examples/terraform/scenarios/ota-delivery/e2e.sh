#!/usr/bin/env bash
# End-to-end driver for the server-less OTA delivery scenario. Provisions the
# ARM estate against azemu, publishes a signed update, promotes it, asserts the
# CDN read path, then tears down. Local-only: it needs the running Azurite data
# plane (the ARM-half smoke that CI runs lives in main.tftest.hcl).
#
# Run via `make ota-delivery` from the repo root (which brings up the stack).
set -euo pipefail

SCENARIO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCENARIO_DIR/../../../.." && pwd)"

CERT="${SSL_CERT_FILE:-$REPO_ROOT/.azemu/cert-bundle.pem}"
AZURITE="${AZEMU_AZURITE_ENDPOINT:-http://127.0.0.1:10000}"
# Well-known Azurite dev key (azemu's listKeys returns it for any account);
# also the key in docker-compose.yml AZURITE_ACCOUNTS.
DEV_KEY="Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw=="

RUNTIME_VERSION=1.0.0
CHANNEL=PRODUCTION
PLATFORM=android
VERSION=1
CONTAINER=ota
PREFIX="$RUNTIME_VERSION/$CHANNEL/$PLATFORM"

FG=(go run ./examples/terraform/scenarios/ota-delivery/fixturegen)

cd "$SCENARIO_DIR"
cleanup() {
  terraform destroy -auto-approve >/dev/null 2>&1 || true
  rm -f "$SCENARIO_DIR/pub.pem"
}
trap cleanup EXIT

echo "== terraform apply =="
terraform init -input=false >/dev/null
terraform apply -auto-approve

ACCOUNT="$(terraform output -raw storage_account_name)"
KID="$(terraform output -raw manifest_key_id)"
FQDN="$(terraform output -raw cdn_endpoint_fqdn)"
terraform output -raw manifest_key_public_pem > "$SCENARIO_DIR/pub.pem"

cd "$REPO_ROOT"

echo "== publish v$VERSION (build, sign with Key Vault, upload immutable artefacts) =="
PUB="$("${FG[@]}" publish -account "$ACCOUNT" -key "$DEV_KEY" -azurite "$AZURITE" \
  -container "$CONTAINER" -prefix "$PREFIX" -version "$VERSION" -kid "$KID" -cacert "$CERT")"
echo "$PUB"
ASSET="$(printf '%s\n' "$PUB" | sed -n 's/^ASSET=//p')"

echo "== promote v$VERSION to 100% (server-side copy, write rollout.json) =="
"${FG[@]}" promote -account "$ACCOUNT" -key "$DEV_KEY" -azurite "$AZURITE" \
  -container "$CONTAINER" -prefix "$PREFIX" -version "$VERSION"

echo "== verify CDN read path (Content-Type, cache TTLs, signature) =="
"${FG[@]}" verify -fqdn "$FQDN" -base "$CONTAINER/$PREFIX" -version "$VERSION" \
  -asset "$ASSET" -pubkey "$SCENARIO_DIR/pub.pem" -cacert "$CERT"

echo "== ota-delivery e2e: PASS =="
