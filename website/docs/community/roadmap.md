# Roadmap -- azemu

Single source of truth for vision, non-goals, resource roster, and milestones.

Last updated: 2026-04-21

---

## TL;DR

**azemu is a local Azure emulator that runs real `terraform apply` from the
official `hashicorp/azurerm` provider with no subscription, no account, and
no network.** One binary. Fidelity-first. Terraform-native.

```bash
docker compose up -d
terraform init && terraform apply -auto-approve
# ... runs against a fake Azure, no login, no cost.
```

## Why azemu

Other projects in this space exist: LocalStack (AWS-first with Azure
support bolted on), miniblue (breadth-first Azure emulator), Azurite
(Microsoft's official Storage-only emulator), and a handful of
service-specific doubles. azemu exists because none of them optimise for
the thing we care about most: **a Terraform CI loop that does not lie**.

The difference shows up in four places:

1. **Fidelity over breadth.** Every resource azemu emulates has to
   round-trip a real `terraform apply` + `terraform destroy` cycle against
   unmodified `azurerm`. Five distinct classifier-class bugs were hit and
   fixed during the first end-to-end run, all documented so the next
   contributor gets the recipe instead of rediscovering them. "It compiles
   in a unit test" is not enough. "The provider does not reject the
   response" is the bar.

2. **Terraform-first, narrow scope.** azemu is not trying to be an Azure
   CLI replacement, an `azcopy` target, or a drop-in for `Az.PowerShell`.
   It is the shortest path from "contributor writes `azurerm` HCL" to
   "state file on disk". Everything else is someone else's project.

3. **Open-source governance from day 1.** `CODE_OF_CONDUCT.md`,
   `SECURITY.md`, `RELEASING.md`, `CODEOWNERS`, `CONTRIBUTING.md`, a
   pinned `pre-commit` + `golangci-lint` + `markdownlint` chain, a
   per-resource parity matrix, a changelog. No "we will add it when we
   hit 100 stars" debt.

4. **Post-mortem discipline.** Every classifier-class bug earns a dated
   entry with symptom, root cause, fix, and regression test. No silent
   fixes. Over time, the regression corpus keeps fidelity honest.

## Guiding principles

1. **Every fidelity claim is backed by a test.** No resource graduates
   from Stub to Full in the [parity matrix](../concepts/parity-matrix.md) without a unit test pinning
   response shape and an integration test walking a real `terraform apply`
   path through the production mux.

2. **One binary. Minimum dependencies.** Go stdlib, `go-chi/chi`,
   `rs/zerolog`, `golang-jwt/jwt`, `google/uuid`. That is the whole
   allow-list. If a feature needs `viper`, `cobra`, `testify`, `gomock`,
   or any framework with a corporate sponsor, we rewrite it by hand.

3. **Contributors before maintainers.** Documentation is written so a
   brand-new contributor can ship their first resource on day one. If a
   workflow requires tribal knowledge that only the maintainer has, it is
   a documentation bug.

4. **Use cases over features.** `examples/terraform/scenarios/` holds
   full-stack use cases (three-tier, AKS workload, static site, ADO
   pipeline). Each scenario is both documentation and integration test.
   A scenario that does not run in CI is deleted.

5. **Ship Docker on day 1.** Everyone uses Docker. The first-impression
   workflow is `docker compose up`, not `go build` or `flox activate`.
   Docker, docker-compose, and a Nix flake ship in Phase 3. The flox
   environment stays as the contributor-side workspace, not the
   first-time-user workspace.

## Non-goals

State these up-front so scope creep has a clear wall to hit.

- **Not an `azcopy` target.** Storage data-plane work is delegated to
  [Azurite](https://learn.microsoft.com/en-us/azure/storage/common/storage-use-azurite),
  shipped as a sidecar in `docker-compose.yml`. azemu owns the Storage
  management plane (ARM) and points `primaryEndpoints` at Azurite. See
  [Design Decision 0001](../resources/design-decisions/0001-delegate-storage-data-plane-to-azurite.md).
  Uploading multi-GB blobs is Azurite's job, not ours.
- **Not a real Kubernetes control plane.** AKS is a management-plane
  stub. If you need pods, run `kind` or `k3d` alongside azemu.
- **Not a LocalStack clone.** No AWS, no Alibaba, no GCP. Azure only.
- **Not an Azure CLI replacement.** `az login`, `az group list`, and
  friends may happen to work against the ARM endpoints, but they are not
  tested and will not gate a release.
- **Not a cost calculator, policy engine, or billing emulator.** Scope
  is provisioning CRUD and async polling. That is it.
- **Not a pipeline runner.** The Azure DevOps bridge ships OIDC token
  issuance and service-connection CRUD so workload-identity-federation
  Terraform CI works. Actual pipeline execution is out of scope.
- **Not `terraform apply` in the cloud.** azemu is a local development
  and CI tool. Running it as a multi-tenant service is possible but not
  a product goal.

## Resource roster

Legend:

| Mark | Meaning |
|---|---|
| **Full** | Real `terraform apply` and `destroy` round-trip green against `hashicorp/azurerm` |
| **Stub** | Management-plane CRUD accepts PUT and returns `Succeeded`, but does not wire the real provisioning contract (e.g. no live runtime, no async polling beyond 202) |
| **None** | Not implemented yet |

### v0.1 (current, shipping now)

| Resource | ARM provider | Fidelity |
|---|---|---|
| `azurerm_resource_group` | `Microsoft.Resources/resourceGroups` | Full |
| `azurerm_virtual_network` | `Microsoft.Network/virtualNetworks` | Full |
| `azurerm_subnet` | `Microsoft.Network/virtualNetworks/subnets` | Full |

### v0.2 (networking + storage + secrets + CDN)

Priority order inside v0.2 is top-down; ship the first row first.

| Resource | ARM provider | Target fidelity | Notes |
|---|---|---|---|
| `azurerm_public_ip` | `Microsoft.Network/publicIPAddresses` | Full | Prerequisite for LB and Application Gateway |
| `azurerm_network_security_group` | `Microsoft.Network/networkSecurityGroups` | Full | Commonly paired with Subnet in real configs |
| `azurerm_lb` (+ backend pool, rule) | `Microsoft.Network/loadBalancers` | Full | The "Load Balancer" item from the roster |
| `azurerm_application_gateway` | `Microsoft.Network/applicationGateways` | Full | The Azure equivalent of "ingress" |
| `azurerm_dns_zone` + record sets | `Microsoft.Network/dnsZones` | Full | Auto-SOA and NS generation on zone create |
| `azurerm_storage_account` | `Microsoft.Storage/storageAccounts` | Full | ARM management plane, `listKeys`, `primaryEndpoints` rewrite. Data plane delegated to Azurite. See [Design Decision 0001](../resources/design-decisions/0001-delegate-storage-data-plane-to-azurite.md). |
| `azurerm_storage_container` | `Microsoft.Storage/storageAccounts/blobServices/containers` | Full | ARM sub-resource CRUD. Blob data plane served by the Azurite sidecar. |
| `azurerm_key_vault` | `Microsoft.KeyVault/vaults` | Full | Management plane plus secrets data plane |
| `azurerm_key_vault_secret` | `...vaults/secrets` | Full | Secrets CRUD |
| `azurerm_cdn_profile` + endpoint | `Microsoft.Cdn/profiles` + `.../endpoints` | Full | The "CDN" item from the roster |

### v0.3 (identity, AKS, Azure DevOps bridge)

| Resource | ARM provider | Target fidelity | Notes |
|---|---|---|---|
| `azurerm_user_assigned_identity` | `Microsoft.ManagedIdentity/userAssignedIdentities` | Full | Required for workload identity federation |
| `azurerm_federated_identity_credential` | `.../federatedIdentityCredentials` | Full | issuer/subject/audience matching |
| `azurerm_kubernetes_cluster` | `Microsoft.ContainerService/managedClusters` | **Stub** | Management-plane only; no live k8s control plane. See non-goals. |
| IMDS token endpoint | `169.254.169.254/metadata/identity/oauth2/token` (host-binding optional) | Full | Pairs with workload identity federation |
| Azure DevOps OIDC issuer | `SYSTEM_OIDCREQUESTURI` compatible endpoint | Full | What Terraform-in-ADO-pipeline actually needs |
| ADO service connection CRUD | `dev.azure.com/{org}/{project}/_apis/serviceendpoint/endpoints` | Full | Minimal surface for `azuredevops` provider workload-identity flows |

### Beyond v0.3 (tracked, not committed)

| Idea | Why it is there |
|---|---|
| Postgres-backed store | Multi-process CI clusters where one azemu serves many runners |
| `azemu` multi-toolchain CLI | Subcommands (`azemu tf`, `azemu pulumi`, `azemu kubectl`, `azemu python`) auto-start the emulator, inject env vars, and exec the underlying tool. One binary, any IaC toolchain. Replaces the shell `scripts/aztf` wrapper. |
| Plugin SDK | Out-of-process resource modules so community can ship providers without forking |
| Native Terraform test framework (`.tftest.hcl`) | First-class support for `terraform test` in the emulator test pyramid |
| Front Door, Traffic Manager | Requested by users once Application Gateway lands |
| Cosmos DB (management + data plane) | The next natural storage type after Blob |
| Event Grid + Service Bus | Eventing story for microservices scenarios |

## Milestones

### v0.1 -- first open-source release

**Target shape:** RG + VNet + Subnet round-trip a real `terraform apply`.
Contributors and sponsors can evaluate the project in one command.

- Phase 0 DONE: bootstrap, `make smoke` green.
- Phase 1 DONE: end-to-end `terraform apply` + `destroy` for RG + VNet + Subnet.
  Five classifier-class bugs fixed and post-mortemed.
- Phase 2 DONE: per-package test coverage targets met. Integration suite
  walks token mint to OIDC discovery to JWKS signature verification end-to-end.
- Phase 2.5 TODO: OIDC/JWKS package ownership cleanup, tags normalisation.
- Phase 3 TODO: DevEx layer. Docker, docker-compose, Nix flake,
  `examples/terraform/` with single-resource files, Makefile polish,
  startup banner.
- Phase 5 TODO: open-source governance. `CONTRIBUTING.md`, `CHANGELOG.md`,
  `CODE_OF_CONDUCT.md`, `SECURITY.md`, `RELEASING.md`, `CODEOWNERS`, CI
  workflow, `goreleaser`, `v0.1.0` tag.

### v0.2 -- networking, storage, secrets, CDN

**Target shape:** a three-tier web app with a load balancer, a storage
account, a CDN profile, and Key Vault secrets all runs end-to-end. First
real "scenarios" land in `examples/terraform/scenarios/`.

- Phase 4: file-backed state + HTTP state API. Prerequisite 4.0 is the
  `store.Put` error-surface sweep so the first disk error cannot silently
  lose a resource.
- Phase 6: DNS zones, Load Balancer, Application Gateway, Public IP, NSG.
- Phase 7: Storage Account + Blob containers, Key Vault secrets, CDN.
- New scenarios in `examples/terraform/scenarios/`:
  `three-tier/`, `static-site/`.
- Helm chart + Kubernetes deploy examples land here. Deferred from v0.1
  on purpose: a chart is worth shipping only once azemu emulates enough
  to make team-shared CI worth the bandwidth.

### v0.3 -- identity, AKS stub, ADO bridge

**Target shape:** a Terraform CI pipeline running inside Azure DevOps,
using workload identity federation, provisions an AKS cluster and a
Managed Identity. Entire loop runs against azemu with zero cloud cost.

- Phase 8: IMDS, workload identity federation, ADO OIDC issuer, ADO
  service connection CRUD, AKS management-plane stub, Managed Identity,
  Federated Identity Credentials.
- New scenarios in `examples/terraform/scenarios/`:
  `aks-workload/`, `ado-pipeline/`.

## Positioning vs existing projects

> This section exists to keep the "why azemu" story sharp. Drift is
> inevitable, so update it any time a comparable project ships a
> meaningful change.

| Project | Scope | Strategy | Where azemu differs |
|---|---|---|---|
| LocalStack | AWS-first, Azure experimental | Breadth across clouds | azemu is Azure-only and Terraform-first |
| miniblue | Azure, 20+ services | Breadth-first, stub-heavy | azemu is depth-first; every resource round-trips real `terraform apply` |
| Azurite | Storage only (official Microsoft) | Data-plane fidelity for one service | azemu covers the management plane across many services and delegates the Storage data plane to Azurite as a docker-compose sidecar. See [Design Decision 0001](../resources/design-decisions/0001-delegate-storage-data-plane-to-azurite.md). |
| Terraform mocks (hand-rolled) | Scenario-specific | Fast but brittle | azemu is reusable, maintained, and documented |
