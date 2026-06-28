# Terraform native test file (requires Terraform 1.6+).
# Runs a full apply cycle against azemu and asserts outputs are populated.
#
# Usage (from repo root):
#   docker compose up -d --build
#   export SSL_CERT_FILE=$PWD/.azemu/cert-bundle.pem
#   cd examples/terraform/scenarios/ota-updates && terraform init && terraform test

run "ota_pipeline_lifecycle" {
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
}
