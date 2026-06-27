# Install

## Docker (recommended)

Requires Docker, Docker Compose, and Terraform 1.6+ or
[OpenTofu](https://opentofu.org) 1.6+.

```bash
# Start azemu (ARM emulator + Azurite sidecar)
docker compose up -d --build

# Trust the self-signed cert for this shell session
export SSL_CERT_FILE=$PWD/.azemu/cert-bundle.pem

# Run Terraform
cd examples/terraform
terraform init
terraform apply -auto-approve
terraform destroy -auto-approve

# Clean up
cd ../..
docker compose down
```

The `SSL_CERT_FILE` export is needed because azemu serves HTTPS with a self-signed
certificate. The cert is generated at first start and persisted at
`.azemu/cert-bundle.pem` (gitignored), so later starts reuse it.

### Prefer OpenTofu?

azemu uses the standard provider protocol, so [OpenTofu](https://opentofu.org)
is a drop-in replacement. Swap `terraform` for `tofu`:

```bash
cd examples/terraform
tofu init
tofu apply -auto-approve
tofu destroy -auto-approve
```

OpenTofu keeps the whole toolchain open source. See
[License & Forking](../community/license.md) for why we recommend it.

## azemu tf wrapper

If you have the `azemu` binary (via `go build` or flox), the `azemu tf`
subcommand starts the emulator if it is not already running, injects the
env vars, and execs Terraform. It is the fastest path for repeated local
iteration.

```bash
azemu tf -chdir=examples/terraform init
azemu tf -chdir=examples/terraform apply -auto-approve
azemu tf -chdir=examples/terraform destroy -auto-approve
```

No manual `export SSL_CERT_FILE` needed; `azemu tf` sets it before every
invocation. The same wrapper exists for other toolchains: `azemu pulumi`,
`azemu kubectl`, and `azemu python`.

## flox (contributor workflow)

[flox](https://flox.dev) is a developer environment manager that pins Go,
Terraform, pre-commit, and helper functions to exact versions. This is the
workflow used by project contributors.

```bash
flox activate         # installs pre-commit hook on first run
azemu-start           # builds azemu, starts it, prints the one-time cert-trust command
ta                    # alias: terraform apply -auto-approve (in test/terraform)
td                    # alias: terraform destroy -auto-approve
azemu-stop
```

`azemu-start` walks you through a one-time `security add-trusted-cert` step on
macOS. The cert persists at `.azemu/cert-bundle.pem` (gitignored), so later
restarts skip the keychain prompt.

See [Setup (Dev Env)](../reference/setup.md) for the full contributor guide,
including manual (non-flox) setup and the environment variables azemu reads at
startup.
