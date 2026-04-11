# AGENTS.md -- azemu

A README for any coding agent (Claude Code, Cursor, Codex, Aider, etc.)
working on this project. See <https://agents.md> for the spec.

## Project

**azemu** is a Terraform-first, open-source local Azure emulator written in
Go. It intercepts the `hashicorp/azurerm` Terraform provider via the
metadata-redirect pattern and serves a subset of the Azure Resource Manager
REST API surface locally, so contributors can run `terraform apply` against
a fake Azure with no subscription and no external auth.

- Module: `github.com/zerodeth/azemu`
- Owner: Sherif Abdalla (ZeroDeth)
- Licence: MIT
- Status: Phase 1 complete. `terraform init && apply && destroy` round-trip
  is green against azurerm v4.x for resource groups, virtual networks, and
  subnets.

Read `docs/ARCHITECTURE.md` for the system design, package layout, and
request flow. Read `docs/PARITY.md` for the full matrix of what is
implemented today.

## Build and test

The project ships a `flox` environment that pins Go, Terraform, pre-commit,
and the supporting tools. Activating it gives you everything at the exact
versions the project is tested against.

```bash
flox activate          # installs pre-commit hook on first run
make build             # go build -o bin/azemu ./cmd/azemu
make test              # go test ./... -v -count=1
make smoke             # build + start + curl smoke test + stop
azemu-start            # flox helper: build, start, print one-time cert trust
ta && td               # flox helpers: terraform apply && destroy
azemu-stop
```

For the manual (non-flox) setup, the environment variables, the persistent
TLS cert path (`AZEMU_CERT_PATH`), and the IPv6 / `localhost` gotcha, read
`docs/SETUP.md`. For known errors and their fixes, read
`docs/TROUBLESHOOTING.md`.

## Conventions

Full reference lives in `docs/CONVENTIONS.md`. The highlights:

- **Go style** (error wrapping with `%w`, structured zerolog, no printf-style
  logging): `docs/CONVENTIONS.md` S1. Also enforced path-scoped by
  `.claude/rules/go-style.md`.
- **ARM API fidelity rules** (api-version required, PUT idempotency,
  DELETE-is-async, HEAD semantics, Azure error format, Azure headers,
  lowercase location, cascade delete, lowercase chi path literals):
  `docs/CONVENTIONS.md` S2. Also enforced path-scoped by
  `.claude/rules/arm-handlers.md` (loads only when touching
  `internal/arm/**/*.go`).
- **Auth fidelity rules** (RS256 JWT claims, OIDC discovery fields, JWKS
  shape, TLS cert persistence via `AZEMU_CERT_PATH`): `docs/CONVENTIONS.md` S3.
- **Testing strategy** and per-package coverage targets:
  `docs/CONVENTIONS.md` S4. Also `.claude/rules/tests.md`.
- **Documentation style** (no em-dashes, no AI-buzzwords, markdownlint
  gotchas): `.claude/rules/docs.md`.

## Branch and commit discipline

- All work on feature branches. The pre-commit hook blocks commits to `main`.
- One logical change per commit. Do not bundle refactors with feature work.
- Do not skip pre-commit hooks (`--no-verify`). Fix the underlying issue
  instead.
- Do not push to remote unless explicitly asked.
- Do not `git push --force`, do not `git reset --hard` shared branches, do
  not amend published commits.

Full before-commit checklist: `docs/CHECKLISTS.md`.

## Safety

- Do not modify `go.mod` / `go.sum` to add dependencies without approval.
- Do not edit `Dockerfile`, `LICENSE`, or `.github/` workflows without
  approval.
- Do not edit linter config (`.pre-commit-config.yaml`, `.markdownlint.yaml`)
  to silence a violation; fix the offending source instead.
- Do not commit secrets, tokens, private keys, or `.env*` files.
- The self-signed TLS cert is generated at runtime. Never commit it. The
  persistent cert bundle lives at `.azemu/cert-bundle.pem` and is gitignored.
- Do not delete files or branches without explicit approval.

## Subagents and orchestration

Subagent role definitions (arm-resource-implementer, test-writer,
code-reviewer, terraform-compatibility-debugger, docs-writer) and
orchestration patterns live in `docs/SUBAGENTS.md`. They are reference
recipes, not auto-loaded behavior. Invoke them explicitly when the shape
of the work matches.

## Project files at a glance

| File | Purpose |
|------|---------|
| `README.md` | Public-facing intro and quickstart |
| `CHANGELOG.md` | Keep-a-changelog release history |
| `TASKS.md` | Phased implementation plan, current status |
| `TODO.md` | Known gaps and post-mortems |
| `CLAUDE.md` | Claude-Code-specific overrides (imports this file) |
| `docs/ARCHITECTURE.md` | Package layout, dependency graph, request flow |
| `docs/CONVENTIONS.md` | Full Go/ARM/auth/testing reference |
| `docs/CHECKLISTS.md` | Add-a-resource, modify-store, before-commit recipes |
| `docs/SUBAGENTS.md` | Subagent role definitions and orchestration patterns |
| `docs/PARITY.md` | Full/Stub/None matrix per resource |
| `docs/SETUP.md` | Contributor onboarding (flox + manual paths) |
| `docs/TROUBLESHOOTING.md` | Common errors and fixes |
| `.claude/rules/*.md` | Path-scoped rules that load only when matching files are touched |
| `.flox/env/manifest.toml` | Pinned dev environment (Go, Terraform, pre-commit, ...) |
| `.pre-commit-config.yaml` | Hygiene + go vet/build + golangci-lint + markdownlint |
