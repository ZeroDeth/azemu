# Terraform native test file (requires Terraform 1.6+).
# Runs a full apply cycle against azemu and asserts outputs are populated.
#
# Usage (from repo root):
#   docker compose up -d --build
#   export SSL_CERT_FILE=$PWD/.azemu/cert-bundle.pem
#   cd examples/terraform/scenarios/redis-cache && terraform init && terraform test

run "redis_cache_with_keyvault_secret" {
  command = apply

  assert {
    condition     = output.resource_group_id != ""
    error_message = "resource_group_id must not be empty"
  }

  assert {
    condition     = output.redis_cache_id != ""
    error_message = "redis_cache_id must not be empty"
  }

  assert {
    condition     = output.redis_hostname != ""
    error_message = "redis_hostname must not be empty"
  }

  assert {
    condition     = output.redis_ssl_port > 0
    error_message = "redis_ssl_port must be greater than zero"
  }

  assert {
    condition     = output.key_vault_secret_id != ""
    error_message = "key_vault_secret_id must not be empty (Key Vault secret round-trip)"
  }
}
