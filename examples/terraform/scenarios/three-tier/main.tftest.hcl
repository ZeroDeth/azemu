# Terraform native test file (requires Terraform 1.6+).
# Runs a full apply cycle against azemu and asserts outputs are populated.
#
# Usage (from repo root):
#   docker compose up -d --build
#   export SSL_CERT_FILE=$PWD/.azemu/cert-bundle.pem
#   cd examples/terraform/scenarios/three-tier && terraform init && terraform test

run "three_tier_lifecycle" {
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
    condition     = output.web_subnet_id != ""
    error_message = "web_subnet_id must not be empty"
  }

  assert {
    condition     = output.app_subnet_id != ""
    error_message = "app_subnet_id must not be empty"
  }

  assert {
    condition     = output.data_subnet_id != ""
    error_message = "data_subnet_id must not be empty"
  }

  assert {
    condition     = output.lb_id != ""
    error_message = "lb_id must not be empty"
  }

  assert {
    condition     = output.lb_public_ip != ""
    error_message = "lb_public_ip must not be empty"
  }

  assert {
    condition     = output.appgw_id != ""
    error_message = "appgw_id must not be empty"
  }
}
