# Design notes

This directory holds the design notes for azemu.

A design note is a short write-up of a notable choice and why we made it,
so a newcomer can understand the reasoning without reading the code. Each
one captures the context that prompted the choice, the alternatives we
weighed, and what we accept as a result. When a decision changes, a newer
note supersedes the old one; we keep the old note around so the history
stays readable.

## Index

| ID | Title | Status |
|----|-------|--------|
| 0001 | [Delegate Storage data plane to Azurite](0001-delegate-storage-data-plane-to-azurite.md) | Implemented |
| 0002 | [azemu + kind hybrid for AKS workload deployments](0002-azemu-plus-kind-for-aks-workload-deployments.md) | Proposed |
| 0003 | [Add Azure Cache for Redis](0003-add-azure-cache-for-redis.md) | Implemented |
| 0004 | [How far azemu emulates Azure DevOps Pipelines](0004-azure-devops-pipelines-scope.md) | Proposed |

## When to write one

Write a design note when a change:

- Picks one approach over a credible alternative for a long-lived
  architectural question (storage model, auth model, interception
  strategy, third-party delegation).
- Reverses or replaces an earlier choice.
- Introduces a new cross-cutting dependency (runtime, sidecar, protocol).

You do not need one for:

- Resource-level implementation choices (those live in `TASKS.md`).
- Bug post-mortems (those live in `TODO.md`).
- Coding conventions (those live in `docs/CONVENTIONS.md` and
  `.claude/rules/`).

## Filename convention

`NNNN-kebab-case-title.md`, zero-padded to four digits, numbered in order.
The next note is `0005-...`.
