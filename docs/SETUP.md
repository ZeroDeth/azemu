# Setup Guide -- azemu

Everything required to get azemu running and talking to Terraform.

## Prerequisites

- Go 1.22+
- [flox](https://flox.dev) (manages project-scoped tooling)
- macOS or Linux

## Development environment (flox)

azemu uses flox for project-scoped dependencies so you do not install tools
system-wide. Terraform lives inside this repo's environment, not on your PATH.

```bash
# Initialise the flox environment (creates .flox/)
flox init -n azemu

# Install terraform
flox install terraform

# Verify
flox activate -c "terraform version"
```

Run terraform commands inside the flox environment:

```bash
# Option A: interactive shell
flox activate
cd test/terraform
terraform init

# Option B: one-shot
flox activate -c "terraform -chdir=test/terraform init"
```

## TLS certificate trust

azemu generates a new self-signed TLS certificate every time it starts. The
certificate path is printed on startup:

```text
INF TLS cert written, export SSL_CERT_FILE to trust it path=/var/folders/.../azemu-cert.pem
```

### Step 1: Export SSL_CERT_FILE

Tools that respect `SSL_CERT_FILE` (curl, some HTTP clients) trust the cert
automatically once exported:

```bash
export SSL_CERT_FILE=/var/folders/.../azemu-cert.pem
```

### Step 2: Trust at the OS level (macOS)

The azurerm Terraform provider uses Go's native TLS stack, which ignores
`SSL_CERT_FILE` and checks the macOS keychain. You must import the cert:

```bash
security add-trusted-cert \
  -d -r trustRoot \
  -k ~/Library/Keychains/login.keychain-db \
  "$SSL_CERT_FILE"
```

On Linux use `update-ca-certificates` or your distribution equivalent.

**Important:** the certificate regenerates on every server restart. You must
re-run the `security add-trusted-cert` command each time azemu restarts, or
keep the server running for your entire session.

## Terraform provider configuration

The provider must use `metadata_host` so it discovers azemu's endpoints instead
of Azure's public cloud URLs.

```hcl
provider "azurerm" {
  features {}

  metadata_host                     = "localhost:4567"
  resource_provider_registrations   = "none"
  subscription_id                   = "00000000-0000-0000-0000-000000000000"
  tenant_id                         = "00000000-0000-0000-0000-000000000001"
  client_id                         = "00000000-0000-0000-0000-000000000002"
  client_secret                     = "azemu-mock-secret"
}
```

Environment variable alternative:

```bash
export ARM_METADATA_HOSTNAME=localhost:4567
export ARM_SUBSCRIPTION_ID=00000000-0000-0000-0000-000000000000
export ARM_TENANT_ID=00000000-0000-0000-0000-000000000001
export ARM_CLIENT_ID=00000000-0000-0000-0000-000000000002
export ARM_CLIENT_SECRET=azemu-mock-secret
```

### Metadata cloud classification

The azurerm provider classifies clouds by inspecting the `/metadata/endpoints`
response. If the `resourceManager` URL uses `http://` instead of `https://`, the
provider classifies the environment as Azure Stack and refuses to connect. azemu
serves ARM endpoints on HTTPS to avoid this rejection.

## Running the server

```bash
make build
./bin/azemu
```

Ports:

- `:4566` (HTTPS) - ARM API, data plane
- `:4567` (HTTPS) - metadata service, OAuth2, OIDC

## Quick validation

```bash
# Metadata endpoint
curl -sk https://localhost:4567/metadata/endpoints?api-version=2022-09-01

# ARM subscriptions
curl -sk https://localhost:4566/subscriptions?api-version=2022-12-01

# Full automated smoke test
make smoke
```

## See also

- [docs/TROUBLESHOOTING.md](TROUBLESHOOTING.md)
- [docs/PARITY.md](PARITY.md)
- [docs/ARCHITECTURE.md](ARCHITECTURE.md)
