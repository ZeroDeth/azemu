# Install

## Docker (recommended)

Requires Docker, Docker Compose, and Terraform 1.6+.

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

## aztf wrapper

The `scripts/aztf` wrapper automates the env-var exports and starts azemu if it
is not already running. It is the fastest path for repeated local iteration.

```bash
./scripts/aztf -chdir=examples/terraform init
./scripts/aztf -chdir=examples/terraform apply -auto-approve
./scripts/aztf -chdir=examples/terraform destroy -auto-approve
```

No manual `export SSL_CERT_FILE` needed; `aztf` sets it before every invocation.

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
