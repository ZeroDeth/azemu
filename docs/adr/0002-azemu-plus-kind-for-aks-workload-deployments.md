# ADR 0002: azemu + kind hybrid for AKS workload deployments

- Status: Proposed
- Date: 2026-04-27
- Deciders: @ZeroDeth
- Supersedes: none

## Context

ROADMAP.md establishes that `azurerm_kubernetes_cluster` is a
**management-plane stub**: azemu accepts the ARM PUT, returns a
provisioned cluster shape, and never runs a real Kubernetes control
plane. The non-goals section is explicit: "Not a real Kubernetes control
plane. AKS is a management-plane stub. If you need pods, run `kind` or
`k3d` alongside azemu."

Real-world deployment tickets exercise more than ARM CRUD. A typical
production AKS workload (the OTA POC ticket ONA-56232 is the trigger for
this ADR) needs:

1. ARM CRUD for the cluster, storage account, blob containers, Key
   Vault, Managed Identity (azemu owns this today).
2. A Kubernetes API server that accepts `kubectl apply`, schedules pods,
   runs liveness probes, and mounts CSI volumes (azemu does **not**
   serve this).
3. Data-plane backends the workload reads and writes: Blob via Azurite
   (already delegated in ADR 0001), Key Vault secrets via the azemu
   secrets data plane, Redis (proposed separately in ADR 0003).
4. Workload-identity federation so a pod's projected service-account
   token is exchanged for an Azure access token tied to a Managed
   Identity (Federated Identity Credential, Phase 8.2 in TASKS.md).

Today contributors who want to validate a full deployment scenario
against azemu hit the gap at step 2 and have no documented path forward.
The ROADMAP's non-goal sentence points at `kind`/`k3d` but the
integration is not specified anywhere a contributor can copy.

## Decision

**Adopt `azemu + kind` as the supported pattern for AKS workload
deployment scenarios. Document it as a first-class hybrid in
`docs/ARCHITECTURE.md` and ship a reference scenario under
`examples/terraform/scenarios/aks-workload/`.**

In concrete terms:

- The AKS resource in azemu remains a management-plane stub. No change
  to its scope. ARM `apply` and `destroy` continue to round-trip green.
- A `kind` cluster (or `k3d`) runs alongside azemu in
  `docker-compose.yml` (or as a sibling `make` target) and provides the
  real Kubernetes control plane for any scenario that needs to schedule
  pods.
- The `aks-workload/` scenario wires the two together: Terraform
  provisions the ARM resources against azemu (cluster stub, storage,
  Key Vault, Managed Identity, Federated Identity Credential), and a
  `kubectl apply` step (run from the same Makefile target) deploys the
  workload manifest to the `kind` cluster.
- The `kind` cluster is configured to trust azemu's TLS cert and resolve
  azemu hostnames inside the cluster network so a pod can reach the
  Azure data planes (Blob via Azurite, Key Vault via azemu, Redis via
  the sidecar from ADR 0003) over the same `metadata_host` intercept
  pattern.
- Workload identity wiring uses the OIDC issuer azemu already serves
  (Phase 8.5) plus the Federated Identity Credential resource (Phase
  8.2). The pod's projected token is signed by `kind`, traded at the
  Azure token endpoint served by azemu, and the resulting access token
  is honoured by the azemu Key Vault secrets data plane.

## Rationale

1. **Keep AKS scope honest.** Implementing a real Kubernetes control
   plane is several orders of magnitude more work than every other
   resource in the roster combined and would compete with `kind`/`k3d`,
   which already do this job well. Rebuilding it inside azemu would
   violate the "do not reimplement what upstream already ships"
   principle that drove ADR 0001.

2. **Separation of concerns matches the cloud.** Real Azure separates
   the AKS control plane (Microsoft's responsibility) from the workload
   the customer deploys. The hybrid mirrors that split: azemu owns the
   ARM control plane, `kind` owns the cluster compute. A scenario that
   only needs ARM-level assertions never has to start `kind`.

3. **Unblocks deployment-shaped tickets.** ONA-56232 (deploy
   expo-open-ota to QA AKS) is the immediate trigger, but every future
   "deploy real workload to AKS" ticket has the same shape: pods, CSI
   volumes, probes, ingress, network policy. Without a documented
   hybrid pattern, each ticket negotiates the same gap.

4. **Reuses azemu's existing intercept pattern.** Pods inside `kind`
   reach Azure data planes the same way the host does: via the
   `metadata_host` redirect the azurerm provider already follows. Only
   the cert trust and DNS resolution step is new, and both are
   well-trodden `kind` configuration knobs.

5. **Cost stays at zero.** No cloud subscription, no AKS bill, and
   `kind` runs on every contributor's existing Docker daemon.

## Consequences

### Positive

- ARM-only scenarios (the existing v0.1 and v0.2 examples) are
  unaffected. They never start `kind`.
- AKS-workload scenarios have a documented, reproducible recipe instead
  of tribal knowledge.
- Deployment-style tickets (CSI mounts, workload identity, network
  policy) are now testable end-to-end without touching real Azure.
- The "AKS is a stub" non-goal in ROADMAP.md becomes a strength, not a
  limitation, because the hybrid is the documented escape hatch.

### Negative

- Two control planes (azemu + `kind`) for scenarios that exercise pods.
  Mitigated: scenarios pick whichever subset they need; the Makefile
  target encapsulates the choreography.
- Cert-trust and DNS-resolution wiring inside `kind` is one more thing
  to document. Captured in the scenario README.
- The hybrid increases cold-start time for `make scenario-aks-workload`
  by however long `kind create cluster` takes (~30-60s on most
  hardware). Acceptable for a deployment-shaped scenario, not for a
  basic ARM round-trip.

### Neutral

- Contributors not working on AKS scenarios install nothing new. `kind`
  is added to the flox manifest only when Phase 8.7 work begins.
- Future replacement of `kind` with `k3d` is a one-flag change in the
  Makefile target. The hybrid is not coupled to a specific local
  Kubernetes runtime.

## Alternatives considered

1. **Implement a real Kubernetes control plane inside azemu.**
   Rejected. Out of scope per ROADMAP non-goals, and `kind` already
   solves this with full kube-apiserver fidelity.

2. **Defer all AKS workload scenarios indefinitely.** Rejected. Real
   tickets (ONA-56232) need this today, and "we are an ARM emulator and
   that is enough" leaves a category of high-value scenarios untested.

3. **Embed `kind` as a subprocess inside the azemu binary.** Rejected.
   Mixes runtimes inside one image, complicates the Dockerfile, and
   loses the clean "optional second runtime" on/off switch that
   sibling-process composition gives us. Same reasoning as ADR 0001's
   alternative 2.

4. **Document the hybrid in a wiki or external blog post.** Rejected.
   Architecture decisions belong in the repo, in version control, next
   to the code they explain. Future contributors should find the
   decision when they grep `docs/`, not after a search engine ranks an
   external page.

## Implementation (Phase 8.7, planned)

The following is the planned scope when `scenarios/aks-workload/` lands:

- `examples/terraform/scenarios/aks-workload/`: Terraform config covering
  RG, VNet, Subnet, AKS stub, Storage Account, Blob container, Key Vault,
  Managed Identity, and Federated Identity Credential. Optional Redis
  cache (per ADR 0003) for scenarios that require it.
- `examples/terraform/scenarios/aks-workload/manifests/`: a deployment
  YAML the `kind` cluster receives. Kept minimal; demonstrates Key Vault
  CSI mount and Blob read.
- `examples/terraform/scenarios/aks-workload/Makefile.scenario` (or
  inclusion in the root Makefile): one target that creates `kind`,
  applies Terraform against azemu, applies the deployment YAML to
  `kind`, runs assertions, and tears both down.
- `examples/terraform/scenarios/aks-workload/README.md`: explains the
  cert-trust and DNS-resolution wiring, the data flow, and the
  assertions.
- Updates to `docs/ARCHITECTURE.md` to add a "hybrid deployment
  scenarios" section with the request-flow diagram for an in-cluster
  pod reaching the Azure data planes.

## Open questions

- **kind vs k3d.** Both are credible. `kind` is more widely deployed in
  Microsoft and CNCF examples; `k3d` starts faster. Phase 8.7 picks one
  on first implementation; the ADR does not bind us.
- **Reuse of `:4566`/`:4567` from inside the `kind` network.** The
  scenario can either expose azemu via host networking or via a
  Kubernetes Service of type ExternalName. The first is simpler; the
  second is closer to a real cluster topology. Decide during Phase 8.7
  build-out.

## References

- ROADMAP.md non-goals: "Not a real Kubernetes control plane."
- TASKS.md Phase 8.4 (`azurerm_kubernetes_cluster` management stub),
  Phase 8.7 (`scenarios/aks-workload/`), Phase 8.2 (Federated Identity
  Credential).
- ADR 0001 (delegate Storage data plane to Azurite): same pattern at
  the Storage layer.
- ADR 0003 (add Azure Cache for Redis): companion proposal for the
  Redis backend the OTA-class scenario needs.
- [kind](https://kind.sigs.k8s.io/), [k3d](https://k3d.io/).
