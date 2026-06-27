# Terraform native test file (requires Terraform 1.6+).
#
# This is the ARM-half smoke: it runs a full apply cycle against azemu and
# asserts the control-plane outputs are populated. It does NOT publish bundles
# or assert the CDN read path; that imperative loop lives in `make ota-delivery`
# (local-only) because it needs the running Azurite data plane and a fixture
# build. CI runs only this tftest, so the scenario stays green in CI without the
# publish/serve steps.
#
# Usage (from repo root):
#   docker compose up -d --build
#   export SSL_CERT_FILE=$PWD/.azemu/cert-bundle.pem
#   cd examples/terraform/scenarios/ota-delivery && terraform init && terraform test

run "ota_delivery_lifecycle" {
  command = apply

  assert {
    condition     = output.resource_group_id != ""
    error_message = "resource_group_id must not be empty"
  }

  assert {
    condition     = output.storage_account_id != ""
    error_message = "storage_account_id must not be empty"
  }

  assert {
    condition     = output.storage_primary_blob_endpoint != ""
    error_message = "storage_primary_blob_endpoint must not be empty"
  }

  assert {
    condition     = startswith(output.key_vault_uri, "https://")
    error_message = "key_vault_uri must be an HTTPS URL"
  }

  assert {
    condition     = strcontains(output.manifest_key_id, "/keys/manifest-signing-key/")
    error_message = "manifest_key_id must be a versioned kid"
  }

  assert {
    condition     = strcontains(output.manifest_key_public_pem, "BEGIN PUBLIC KEY")
    error_message = "manifest_key_public_pem must be a PEM public key"
  }

  assert {
    condition     = endswith(output.cdn_endpoint_fqdn, ".azureedge.net")
    error_message = "cdn_endpoint_fqdn must be an azureedge.net host"
  }
}
