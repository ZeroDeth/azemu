# Terraform native test file (requires Terraform 1.6+).
# Runs a full apply cycle against azemu and asserts outputs are populated.
#
# Usage (from repo root):
#   docker compose up -d --build
#   export SSL_CERT_FILE=$PWD/.azemu/cert-bundle.pem
#   cd examples/terraform/scenarios/static-site && terraform init && terraform test

run "static_site_lifecycle" {
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
    condition     = output.cdn_profile_id != ""
    error_message = "cdn_profile_id must not be empty"
  }

  assert {
    condition     = endswith(output.cdn_endpoint_fqdn, ".azurefd.net")
    error_message = "cdn_endpoint_fqdn must be a Front Door .azurefd.net host"
  }

  assert {
    condition     = output.dns_zone_id != ""
    error_message = "dns_zone_id must not be empty"
  }

  assert {
    condition     = output.www_cname_fqdn != ""
    error_message = "www_cname_fqdn must not be empty"
  }
}
