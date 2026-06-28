# Design note 4: How far azemu emulates Azure DevOps Pipelines

<div class="designnote-meta">
<span class="designnote-meta-item"><span class="designnote-status designnote-status--proposed">Proposed</span></span>
<span class="designnote-meta-item"><strong>Date</strong> 2026-06-27</span>
<a href="https://github.com/ZeroDeth/azemu/blob/main/docs/design-notes/0004-azure-devops-pipelines-scope.md" class="designnote-github-link">Full text on GitHub →</a>
</div>

<div class="designnote-decision" markdown>

<span class="designnote-decision-label">▸ DECISION</span>

**Stay at the auth-and-provisioning boundary azemu has today, and treat a
read-only Pipelines run API as demand-driven: add it only when a real scenario
needs to read pipeline or build state. Do not build a local pipeline runner.**

- azemu already serves the part that is awkward without a cloud: the ADO OIDC
  token issuer on `:4569` and Service Connections CRUD. A pipeline federates
  into azemu and provisions and uses Azure resources with no subscription.
- A pipeline runner is the expensive part, and Microsoft already open-sources
  the Azure Pipelines agent. Rebuilding it inside azemu would be the largest
  component in the project and would duplicate the thing it copies.
- This is the same boundary design note 2 drew for Kubernetes: azemu is the
  Azure a pipeline talks to, not the pipeline. The pipeline's steps run
  directly against azemu, as the `ado-pipeline` and `ota-delivery` scenarios do.

</div>

## Consequences

### Positive

- No new always-on runtime, no agent protocol, no task ecosystem to maintain.
- Pipeline-shaped scenarios already work through the auth and provisioning
  surface (`ado-pipeline`, `ota-delivery`).
- The boundary is easy to explain: azemu is the Azure a pipeline talks to, not
  the pipeline.

### Trade-offs

- You cannot, today, point a Pipelines-API client at azemu and get run or build
  state back. If that becomes a common ask, a read-only run API is the answer,
  and this note is the place that said so.
- "Run my whole pipeline locally against azemu" is not a single command. Run
  the pipeline's steps directly, or point a real agent at azemu. The
  `ota-delivery` scenario shows the shape.

### Neutral

- Contributors who only need ARM and the OIDC federation install nothing new.
- The existing ADO files (`internal/ado/`) are unaffected; this note documents
  a boundary, it does not change code.
