# Terraform native test file (requires Terraform 1.6+).
# Runs a full apply cycle against azemu and asserts outputs are populated.
#
# Usage:
#   docker compose up -d --build
#   export SSL_CERT_FILE=$PWD/.azemu/cert-bundle.pem
#   cd examples/terraform && terraform test

run "full_lifecycle" {
  command = apply

  assert {
    condition     = output.resource_group_id != ""
    error_message = "resource_group_id must not be empty"
  }

  assert {
    condition     = output.vnet_id != ""
    error_message = "vnet_id must not be empty"
  }

  assert {
    condition     = output.subnet_id != ""
    error_message = "subnet_id must not be empty"
  }

  assert {
    condition     = output.redis_cache_id != ""
    error_message = "redis_cache_id must not be empty"
  }

  assert {
    condition     = output.key_vault_key_id != ""
    error_message = "key_vault_key_id must not be empty"
  }
}
