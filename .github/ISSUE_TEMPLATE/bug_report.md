---
name: Bug report
about: Something is broken or behaving unexpectedly
title: ''
labels: bug
assignees: ''
---

## Versions

- **azemu**: (commit hash or tag, e.g. `v0.1.0` or `aae0ed7`)
- **azurerm provider**: (e.g. `4.68.0`)
- **Terraform**: (e.g. `1.14.1`)
- **OS**: (e.g. macOS 15.4, Ubuntu 24.04)

## What happened?

<!-- A clear description of the bug. -->

## Steps to reproduce

<!--
1. Start azemu with `...`
2. Run `terraform apply` with this config: ...
3. See error
-->

## Expected behaviour

<!-- What you expected to happen instead. -->

## Error output

<!--
Paste the full error output. Include the Terraform error and, if
applicable, the azemu server log.
-->

```text
```

## `/api/unhandled` output

<!--
Run: curl -sk https://localhost:4566/api/unhandled
This shows any ARM routes azemu received but could not handle.
If empty, say "empty".
-->

```json
```

## Additional context

<!-- Anything else: screenshots, config snippets, related issues. -->
