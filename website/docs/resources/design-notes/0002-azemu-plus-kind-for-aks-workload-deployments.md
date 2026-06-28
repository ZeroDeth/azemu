# Design note 2: azemu + kind hybrid for AKS workload deployments

<div class="designnote-meta">
<span class="designnote-meta-item"><span class="designnote-status designnote-status--proposed">Proposed</span></span>
<span class="designnote-meta-item"><strong>Date</strong> 2026-04-28</span>
<a href="https://github.com/ZeroDeth/azemu/blob/main/docs/design-notes/0002-azemu-plus-kind-for-aks-workload-deployments.md" class="designnote-github-link">Full text on GitHub →</a>
</div>

<div class="designnote-decision" markdown>

<span class="designnote-decision-label">▸ DECISION</span>

**Adopt `azemu + kind` as the supported pattern for AKS workload deployment
scenarios. Document it as a first-class hybrid and ship a reference scenario
under `examples/terraform/scenarios/aks-workload/`.**

- The AKS resource in azemu remains a management-plane stub. ARM `apply` and
  `destroy` continue to round-trip green.
- A `kind` cluster runs alongside azemu and provides the real Kubernetes
  control plane for scenarios that need to schedule pods.
- The `aks-workload/` scenario wires both: Terraform provisions ARM resources
  against azemu; a `kubectl apply` step deploys the workload to `kind`.
- The `kind` cluster trusts azemu's TLS cert and resolves azemu hostnames
  inside the cluster network, so pods reach Blob (Azurite), Key Vault, and
  Redis (design note 3) over the same `metadata_host` intercept.
- Workload identity uses azemu's OIDC issuer (Phase 8.5) and Federated
  Identity Credential resource (Phase 8.2).

</div>

## Consequences

### Positive

- ARM-only scenarios (existing v0.1 and v0.2 examples) are unaffected.
- AKS-workload scenarios have a documented, reproducible recipe.
- Deployment-style scenarios (CSI mounts, workload identity, network policy)
  are testable end-to-end without touching real Azure.
- The "AKS is a stub" non-goal becomes a strength: the hybrid is the
  documented escape hatch.

### Negative

- Two control planes (azemu + `kind`) for pod-exercising scenarios. Mitigated
  by Makefile targets that encapsulate the choreography.
- Cert-trust and DNS-resolution wiring inside `kind` needs documentation.
  Captured in the scenario README.
- Cold-start for `make scenario-aks-workload` increases by ~30-60 s for
  `kind create cluster`. Acceptable for a deployment-shaped scenario.

### Neutral

- Contributors not working on AKS scenarios install nothing new.
- Future replacement of `kind` with `k3d` is a one-flag change in the
  Makefile target.

## Open questions

- **kind vs k3d.** Phase 8.7 picks one on first implementation.
- **Reuse of `:4566`/`:4567` from inside the `kind` network.** Host networking
  or a Kubernetes Service of type ExternalName. Decide during Phase 8.7.
