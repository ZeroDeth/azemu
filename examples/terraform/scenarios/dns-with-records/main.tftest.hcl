# Terraform native test file (requires Terraform 1.6+).
# Runs a full apply cycle against azemu and asserts outputs are populated.
#
# Usage (from repo root):
#   docker compose up -d --build
#   export SSL_CERT_FILE=$PWD/.azemu/cert-bundle.pem
#   cd examples/terraform/scenarios/dns-with-records && terraform init && terraform test

run "dns_zone_with_records" {
  command = apply

  assert {
    condition     = output.resource_group_id != ""
    error_message = "resource_group_id must not be empty"
  }

  assert {
    condition     = output.dns_zone_id != ""
    error_message = "dns_zone_id must not be empty"
  }

  assert {
    condition     = length(output.dns_zone_name_servers) > 0
    error_message = "dns_zone_name_servers must be populated"
  }

  assert {
    condition     = output.a_record_id != ""
    error_message = "a_record_id must not be empty"
  }

  assert {
    condition     = output.cname_record_fqdn != ""
    error_message = "cname_record_fqdn must not be empty"
  }
}
