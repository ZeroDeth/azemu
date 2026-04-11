# Setup Guide -- azemu

Everything required to get azemu running and talking to Terraform.

## Prerequisites

- [flox](https://flox.dev) (the only host-level requirement; pulls everything else)
- macOS or Linux

## Development environment (flox)

The repo ships a fully pinned `.flox/env/manifest.toml`. Activating it gives
you Go, Terraform `^1.14`, pre-commit, jq, just, shellcheck and tflint at
exactly the versions the project is tested against. You do not need to install
any of these system-wide.

```bash
flox activate
```

The activation hook runs once per environment and:

1. Creates `.azemu/` for the persistent TLS cert bundle.
2. Installs `.git/hooks/pre-commit` from `.pre-commit-config.yaml` if it isn't already.
3. Reports if azemu is already running with a persistent cert.

Helper functions provided by the profile:

| Function | What it does |
|---|---|
| `azemu-start` | Builds, starts the binary, prints the one-time cert-trust command, probes `/metadata/endpoints` |
| `azemu-stop` | `pkill -f bin/azemu` |
| `azemu-status` | Reports running/stopped and dumps the `name` and `resourceManager` fields from `/metadata/endpoints` |
| `azemu-smoke` | Inline smoke test against a running instance |
| `tf-init`, `tf-plan`, `tf-apply`, `tf-destroy` | `terraform -chdir=$TF_DIR ...` against azemu (with `azemu-status` precheck) |
| Aliases: `ti`, `tp`, `ta`, `td`, `ts` | Short forms of the above |

## Environment variables

Sourced from `.flox/env/manifest.toml [vars]` and `pkg/config/config.go`:

| Variable | Required | Default | Purpose |
|---|---|---|---|
| `AZEMU_ARM_PORT` | No | `4566` | ARM HTTPS port (informational; binary is hard-coded today) |
| `AZEMU_META_PORT` | No | `4567` | Metadata HTTPS port (informational) |
| `AZEMU_CERT_PATH` | No | unset | When set, persist the self-signed cert+key as a PEM bundle at this path; trust once and restart freely. When unset, a fresh cert is generated and written to OS temp on every startup. |
| `AZEMU_SUBSCRIPTION_ID` | No | `00000000-0000-0000-0000-000000000000` | Mock subscription returned by ARM list endpoints |
| `AZEMU_TENANT_ID` | No | `00000000-0000-0000-0000-000000000001` | Mock tenant returned by token / OIDC endpoints |
| `AZEMU_METADATA_HOST` | No | `localhost:4567` | Host substituted into URLs in `/metadata/endpoints` |
| `ARM_METADATA_HOSTNAME` | Yes (Terraform) | flox sets `127.0.0.1:4567` | Tells `azurerm` to discover endpoints via azemu instead of real Azure |
| `ARM_SUBSCRIPTION_ID` / `ARM_TENANT_ID` / `ARM_CLIENT_ID` / `ARM_CLIENT_SECRET` | Yes (Terraform) | flox sets all four | Mock credentials; azemu accepts any value |
| `TF_DIR` | No | `test/terraform` | Working directory for the `tf-*` profile aliases |

## TLS certificate trust

azemu serves both ports over HTTPS using a self-signed ECDSA P-256 certificate
with SANs for `localhost` and `127.0.0.1`. There are two modes.

### Persistent (recommended)

Set `AZEMU_CERT_PATH` to a stable PEM bundle file. azemu loads the cert+key
from there on startup, or generates and writes a fresh pair (mode `0600`) if
the file does not exist or fails validation. The flox profile defaults this to
`.azemu/cert-bundle.pem` and the directory is gitignored.

```bash
mkdir -p .azemu
AZEMU_CERT_PATH=$PWD/.azemu/cert-bundle.pem ./bin/azemu
```

Trust the bundle once in the system keychain — subsequent restarts reuse the
same cert and keychain prompt does not return:

```bash
# macOS — TouchID/password prompt fires once
security add-trusted-cert -r trustRoot -p ssl \
  -k ~/Library/Keychains/login.keychain-db \
  .azemu/cert-bundle.pem

# Linux
sudo cp .azemu/cert-bundle.pem /usr/local/share/ca-certificates/azemu.crt
sudo update-ca-certificates
```

`azemu-start` (provided by the flox profile) prints the exact macOS command on
first run, scoped to your bundle path.

### Ephemeral (legacy)

If `AZEMU_CERT_PATH` is unset, azemu generates a fresh cert on every startup
and writes a cert-only file to OS temp. The path is logged on startup:

```text
INF TLS cert written, export SSL_CERT_FILE to trust it path=/var/folders/.../azemu-cert.pem
```

You must re-run `security add-trusted-cert` after every restart in this mode,
because the Go-based azurerm provider checks the macOS keychain (it ignores
`SSL_CERT_FILE`). This is why the persistent mode exists; prefer it.

## Terraform provider configuration

The provider must use `metadata_host` so it discovers azemu's endpoints instead
of Azure's public cloud URLs.

```hcl
provider "azurerm" {
  features {}

  metadata_host                   = "127.0.0.1:4567"
  resource_provider_registrations = "none"
  subscription_id                 = "00000000-0000-0000-0000-000000000000"
  tenant_id                       = "00000000-0000-0000-0000-000000000001"
  client_id                       = "00000000-0000-0000-0000-000000000002"
  client_secret                   = "azemu-mock-secret"
}
```

Environment variable alternative (the flox profile exports these for you):

```bash
export ARM_METADATA_HOSTNAME=127.0.0.1:4567
export ARM_SUBSCRIPTION_ID=00000000-0000-0000-0000-000000000000
export ARM_TENANT_ID=00000000-0000-0000-0000-000000000001
export ARM_CLIENT_ID=00000000-0000-0000-0000-000000000002
export ARM_CLIENT_SECRET=azemu-mock-secret
```

> Use `127.0.0.1`, not `localhost`. macOS resolves `localhost` to `::1` first
> and azemu listens on IPv4 — Terraform will fail with `dial tcp [::1]:4567:
> connection refused` otherwise. Also note that `skip_provider_registration`
> is deprecated in azurerm v4.x and silently ignored; use the
> `resource_provider_registrations` form above.

### Metadata cloud classification

The azurerm provider classifies clouds by inspecting the `/metadata/endpoints`
response. If the `resourceManager` URL uses `http://` instead of `https://`, the
provider classifies the environment as Azure Stack and refuses to connect. azemu
serves ARM endpoints on HTTPS to avoid this rejection.

## Running the server

```bash
make build
./bin/azemu                                   # ephemeral cert
# or
mkdir -p .azemu
AZEMU_CERT_PATH=$PWD/.azemu/cert-bundle.pem ./bin/azemu  # persistent cert
```

Ports:

- `:4566` (HTTPS) — ARM API, data plane
- `:4567` (HTTPS) — metadata service, OAuth2, OIDC

## Available make targets

Sourced from `Makefile`:

| Target | Description |
|---|---|
| `make build` | `go build -o bin/azemu ./cmd/azemu` |
| `make run` | `make build && ./bin/azemu` |
| `make test` | `go test ./... -v -count=1` |
| `make smoke` | Build, start server, run inline curl smoke test, stop server |
| `make docker` | Build the Docker image as `azemu:latest` |
| `make docker-run` | Build and run the image with ports `4566`/`4567` exposed |
| `make clean` | Remove `bin/` |

## Quick validation

```bash
# Metadata endpoint
curl -sk https://127.0.0.1:4567/metadata/endpoints?api-version=2022-09-01

# ARM subscriptions
curl -sk https://127.0.0.1:4566/subscriptions?api-version=2022-12-01

# Full automated smoke test
make smoke

# End-to-end against the real azurerm provider
ta && td   # tf-apply && tf-destroy (flox aliases)
```

## See also

- [docs/TROUBLESHOOTING.md](TROUBLESHOOTING.md)
- [docs/PARITY.md](PARITY.md)
- [docs/ARCHITECTURE.md](ARCHITECTURE.md)
