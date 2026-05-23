# Terraform native test file (requires Terraform 1.6+).
# Runs a full apply cycle against azemu and asserts outputs are populated.
#
# Usage (from repo root):
#   docker compose up -d --build
#   export SSL_CERT_FILE=$PWD/.azemu/cert-bundle.pem
#   cd examples/terraform/scenarios/aks-workload && terraform init && terraform test

run "aks_workload_lifecycle" {
  command = apply

  assert {
    condition     = output.resource_group_id != ""
    error_message = "resource_group_id must not be empty"
  }

  assert {
    condition     = output.aks_cluster_id != ""
    error_message = "aks_cluster_id must not be empty"
  }

  assert {
    condition     = output.aks_fqdn != ""
    error_message = "aks_fqdn must not be empty"
  }

  assert {
    condition     = output.identity_id != ""
    error_message = "identity_id must not be empty"
  }

  assert {
    condition     = output.identity_client_id != ""
    error_message = "identity_client_id must not be empty"
  }

  assert {
    condition     = output.key_vault_id != ""
    error_message = "key_vault_id must not be empty"
  }

  assert {
    condition     = output.key_vault_uri != ""
    error_message = "key_vault_uri must not be empty"
  }

  assert {
    condition     = output.secret_id != ""
    error_message = "secret_id must not be empty"
  }
}
