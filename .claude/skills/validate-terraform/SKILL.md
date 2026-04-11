---
name: validate-terraform
description: Run a full `terraform apply` + `terraform destroy` cycle against azemu and check for unhandled routes. Use after ARM, metadata, middleware, or auth changes to confirm the real azurerm v4.x provider still works end-to-end.
---

# End-to-end Terraform validation

Use this skill after any change that touches the ARM surface, the
metadata service, the middleware stack, or the auth endpoints. It runs
the real `hashicorp/azurerm` v4.x provider against azemu and verifies
that `apply` and `destroy` both succeed.

## Prerequisites

- `flox activate` has been run in the current shell (installs pinned
  Go, Terraform, pre-commit, etc.).
- The persistent TLS cert bundle at `.azemu/cert-bundle.pem` has been
  trusted in the system keychain once. If this is a fresh machine,
  `azemu-start` prints the one-time `security add-trusted-cert`
  command.

## Run the loop

```bash
# Inside flox:
azemu-start        # builds, starts azemu, prints cert trust command (once)
ta                 # terraform apply -auto-approve in test/terraform
td                 # terraform destroy -auto-approve
azemu-stop
```

Both `ta` and `td` must exit 0. If either fails, delegate the failure
to the `terraform-compatibility-debugger` subagent.

## Check for unhandled routes

After the run, check whether azemu saw any requests it did not route.

```bash
curl -sk https://127.0.0.1:4566/api/unhandled
```

- Green: `{"unhandled_routes":null}` or an empty list.
- Any entries: new gaps. Record each in `TODO.md` under "Unhandled
  Endpoints (discovered during terraform apply)" with the method,
  path, caller, and whether it blocked apply.

## Manual fallback (outside flox)

If flox is not available:

```bash
mkdir -p .azemu
AZEMU_CERT_PATH=$PWD/.azemu/cert-bundle.pem ./bin/azemu &
sleep 2

cd test/terraform
export ARM_METADATA_HOSTNAME=127.0.0.1:4567
export ARM_SUBSCRIPTION_ID=00000000-0000-0000-0000-000000000000
export ARM_TENANT_ID=00000000-0000-0000-0000-000000000001
export ARM_CLIENT_ID=00000000-0000-0000-0000-000000000002
export ARM_CLIENT_SECRET=azemu-mock-secret

terraform init
terraform apply -auto-approve
terraform destroy -auto-approve

pkill -f 'bin/azemu'
```

Use `127.0.0.1`, not `localhost`: macOS resolves `localhost` to `::1`
first, and azemu listens on IPv4.

## Expected outcomes

- `terraform apply` creates the resources declared in
  `test/terraform/main.tf` (resource group, virtual network, subnet).
- `terraform destroy` removes them cleanly.
- The `/api/unhandled` endpoint returns an empty list.
- No new entries in azemu logs matching `ERROR`.

If any of these fail, the change under test regressed Terraform
compatibility. Do not merge until fixed.
