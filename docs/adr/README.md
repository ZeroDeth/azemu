# Architecture Decision Records

This directory holds Architecture Decision Records (ADRs) for azemu.

An ADR captures a significant architectural choice, the context that
forced the choice, the alternatives considered, and the consequences we
accept. ADRs are immutable: once accepted, an ADR is not edited. If a
decision changes, a new ADR supersedes the old one.

Format follows a light variant of [MADR](https://adr.github.io/madr/).

## Index

| ID | Title | Status |
|----|-------|--------|
| 0001 | [Delegate Storage data plane to Azurite](0001-delegate-storage-data-plane-to-azurite.md) | Implemented |
| 0002 | [azemu + kind hybrid for AKS workload deployments](0002-azemu-plus-kind-for-aks-workload-deployments.md) | Proposed |
| 0003 | [Add Azure Cache for Redis](0003-add-azure-cache-for-redis.md) | Implemented |
| 0004 | [How far azemu emulates Azure DevOps Pipelines](0004-azure-devops-pipelines-scope.md) | Proposed |

## When to write an ADR

Write an ADR when a change:

- Picks one approach over a credible alternative for a long-lived
  architectural question (storage model, auth model, interception
  strategy, third-party delegation).
- Reverses or replaces an earlier architectural decision.
- Introduces a new cross-cutting dependency (runtime, sidecar, protocol).

Do not write an ADR for:

- Resource-level implementation choices (those live in `TASKS.md`).
- Bug post-mortems (those live in `TODO.md`).
- Coding conventions (those live in `docs/CONVENTIONS.md` and
  `.claude/rules/`).

## Filename convention

`NNNN-kebab-case-title.md`, zero-padded to four digits, allocated in
order. The next ADR is `0005-...`.
