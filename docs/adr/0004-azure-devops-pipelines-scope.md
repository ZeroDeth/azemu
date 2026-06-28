# ADR 0004: How far azemu emulates Azure DevOps Pipelines

- Status: Proposed
- Date: 2026-06-27
- Deciders: @ZeroDeth
- Supersedes: none

## Context

azemu already emulates the Azure DevOps surface that a pipeline
*authenticates and provisions through*:

- The ADO OIDC token issuer on `:4569` (`internal/ado/oidc.go`). This is the
  `SYSTEM_OIDCREQUESTURI` endpoint a pipeline calls to trade its job token for
  a federated Azure token, plus the matching
  `/.well-known/openid-configuration` and `/discovery/keys`.
- Service Connections CRUD (`internal/ado/serviceconnection.go`):
  `/{org}/{project}/_apis/serviceendpoint/endpoints`.
- The `ado-pipeline` example scenario, which provisions the Azure-side targets
  a pipeline writes to (a user-assigned identity, a federated credential, a
  Key Vault and secret, a storage account and container).

What azemu does not have is any emulation of the Azure DevOps Pipelines or
Build *run* API (`_apis/pipelines`, `_apis/build/builds`: list a pipeline,
queue a run, get a build, fetch logs), and no runner that executes the steps
in an `azure-pipelines.yml`.

So today a real pipeline (running in hosted Azure DevOps, or on a self-hosted
agent) can federate into azemu and provision and use Azure resources, but
azemu cannot stand in for the pipeline service itself: you cannot point a tool
at azemu and say "queue build 42 and give me its logs".

The question this ADR settles: should azemu grow a pipeline run API, or a
runner, or stay at the auth-and-provisioning boundary it has now?

## The options

Three honest choices, from least to most work.

| Option | What azemu would add | Roughly how much work | What it buys you | Reimplements a runtime upstream already ships? |
|--------|----------------------|-----------------------|------------------|------------------------------------------------|
| **A. Stay at the auth boundary (today)** | Nothing new. OIDC issuer + service connections only. | None | A pipeline federates into a fake Azure and provisions/uses resources with no real subscription. | No |
| **B. Emulate the run API, no execution** | `_apis/pipelines` and `_apis/build/builds` returning believable list/queue/get/logs responses, with runs that report `queued` then `succeeded`. No steps actually run. | Medium | Tools that *read* pipeline state (a dashboard, `az pipelines run --open`, a status badge) get plausible answers offline. | No (it is an API shape, not a runner) |
| **C. Run the YAML locally** | A parser and executor for `azure-pipelines.yml` steps, an agent pool, task resolution, logging. An `act`-for-Azure-DevOps. | Large, open-ended | `make pipeline` could execute a real pipeline against azemu end to end with no cloud. | Yes. This is the Azure Pipelines agent, which Microsoft already ships and open-sources. |

## Decision

**Stay at Option A as the default, and treat Option B as demand-driven: add
the run API only when a real scenario needs to read pipeline or build state.
Do not build Option C.**

This is the same call ADR 0002 made for Kubernetes: azemu is a management and
data-plane emulator for Azure, not a re-implementation of the compute runtimes
that sit on top of it. There, "azemu does not run pods, use `kind`". Here,
"azemu does not run pipeline steps, use the real Azure Pipelines agent, or
just run the steps directly in a shell". The pipeline's *job* is to call
Azure; azemu's job is to be a convincing Azure for it to call.

Concretely:

- A contributor who wants to test a pipeline's Azure interactions points the
  pipeline (or the steps it would run) at azemu via the metadata redirect and
  the OIDC issuer, exactly as the `ado-pipeline` and `ota-delivery` scenarios
  do. The signing, upload, and promotion in `ota-delivery` are the pipeline's
  *work*, run directly against azemu, without a pipeline service in the middle.
- If and when someone has a real need to read run or build state offline (for
  example a tool that polls `_apis/build/builds` for a green run before doing
  something else), that is the trigger to build Option B, scoped to exactly the
  endpoints that need answering. We add it because a user asked, not ahead of
  demand.

## Why

1. **The runner is the expensive part, and it already exists.** A real
   pipeline executor is a large, long-lived piece of software with its own
   security surface, task ecosystem, and agent protocol. Microsoft open-sources
   the agent; rebuilding it inside azemu would be the single biggest component
   in the project and would compete with the thing it copies. Same reasoning as
   ADR 0001 (delegate Blob to Azurite) and ADR 0002 (delegate pods to `kind`).

2. **The auth boundary is the part that actually blocks people.** The genuinely
   awkward thing to do without a cloud is the OIDC federation handshake: minting
   a job token, trading it for an Azure token a service connection trusts. That
   is exactly what azemu already serves, and it is what unblocks pipeline-shaped
   scenarios. The run API on top is mostly bookkeeping.

3. **It keeps the project tool-first and demand-driven.** azemu emulates plain
   Azure and lets real tools drive it; coverage grows from real feature
   requests, not a pre-drawn master plan. A speculative pipeline runner nobody
   asked for is the opposite of that.

4. **Option B stays cheap and available.** Because B is just an API shape over
   the store (the same pattern as every ARM resource), it is a small,
   well-understood addition the day a scenario needs it. Choosing A now does not
   close the door on B later.

## Consequences

### Good

- No new always-on runtime, no agent protocol, no task ecosystem to maintain.
- Pipeline-shaped scenarios already work today through the auth and
  provisioning surface (`ado-pipeline`, `ota-delivery`).
- The boundary is easy to explain to a newcomer: azemu is the Azure a pipeline
  talks to, not the pipeline.

### Trade-offs

- You cannot, today, point a Pipelines-API client at azemu and get run or build
  state back. If that turns out to be a common ask, Option B is the answer, and
  this ADR is the place that said so.
- "Run my whole pipeline locally against azemu" is not a single command. The
  honest substitute is to run the pipeline's steps directly, or to point a real
  agent at azemu. The `ota-delivery` scenario shows the shape.

### Neutral

- Contributors who only need ARM and the OIDC federation install nothing new.
- The existing ADO files (`internal/ado/`) are unaffected; this ADR documents a
  boundary, it does not change code.

## Alternatives considered

1. **Build the run API now (Option B), pre-emptively.** Rejected for timing,
   not merit. It is the right next step *when a scenario needs it*; building it
   before then adds surface with no user behind it.

2. **Build a local runner (Option C).** Rejected. Out of scope for an Azure
   emulator, and it duplicates the open-source Azure Pipelines agent. If you
   want a pipeline to really run locally, run the agent or the steps; azemu is
   there for the Azure calls those steps make.

3. **Drop the ADO surface entirely and tell people to use real Azure DevOps.**
   Rejected. The OIDC federation handshake is precisely the thing that is
   painful without a cloud, and it is cheap for azemu to serve. Keeping it is
   most of the value for least of the cost.

## References

- ROADMAP.md non-goals and the tool-first, demand-driven framing.
- ADR 0001 (delegate the Storage data plane to Azurite): same "do not rebuild
  what upstream ships" principle at the storage layer.
- ADR 0002 (azemu + `kind` for AKS workloads): the same boundary for the
  Kubernetes runtime; this ADR is its Azure DevOps analogue.
- `internal/ado/oidc.go`, `internal/ado/serviceconnection.go`: the ADO surface
  azemu serves today.
- `examples/terraform/scenarios/ado-pipeline/`,
  `examples/terraform/scenarios/ota-delivery/`: pipeline-shaped scenarios that
  run the work directly against azemu.
- Azure Pipelines agent (open source): the runner this ADR declines to
  re-implement.
