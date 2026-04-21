# Contributing to azemu

Thanks for your interest in azemu. This document covers how to add a
resource, the test bar every change has to clear, and the PR checklist.

If you are new here, start with:

- [`README.md`](README.md) for the project pitch and the Docker quick-start.
- [`AGENTS.md`](AGENTS.md) for the cross-vendor agent spec and the pointers
  to per-area rules.
- [`docs/SETUP.md`](docs/SETUP.md) for the flox contributor workflow.
- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) for the request flow and
  package layout.
- [`docs/CONVENTIONS.md`](docs/CONVENTIONS.md) for Go style, ARM contract
  rules, auth fidelity rules, and testing strategy.
- [`docs/PARITY.md`](docs/PARITY.md) for what is implemented and the
  Proof-linked test for every `Full` row.

## Ground rules

- **All work on feature branches.** The pre-commit hook blocks commits to
  `main`.
- **One logical change per commit.** Do not bundle refactors with feature
  work.
- **Do not skip pre-commit hooks** (`--no-verify`). Fix the underlying
  issue instead.
- **Do not push to `main` directly.** Open a pull request.
- **Do not commit secrets, tokens, private keys, or `.env*` files.** The
  self-signed TLS cert is generated at runtime and gitignored.
- **Do not add dependencies** to `go.mod` / `go.sum` without approval in
  the PR description.

## Dev environment

The project ships a [flox](https://flox.dev) environment that pins Go,
Terraform, pre-commit, and supporting tools. Activating it gives you
everything at the exact versions the project is tested against.

```bash
flox activate          # installs pre-commit hook on first run
make build             # go build -o bin/azemu ./cmd/azemu
make test              # go test ./... -v -count=1
make smoke             # build + start + curl smoke test + stop
```

See [`docs/SETUP.md`](docs/SETUP.md) for the manual (non-flox) path, the
`AZEMU_CERT_PATH` environment variable, and the IPv6 / `localhost` gotcha.

## Adding a new ARM resource

This is the most common type of contribution. The full step-by-step
checklist lives in the [`add-resource` skill](.claude/skills/add-resource/SKILL.md).
The short version:

1. Create `internal/arm/{resource}.go` with CRUD + HEAD handlers following
   the pattern in `internal/arm/vnet.go` (not `resourcegroup.go`, which
   predates the shared helpers).
2. Register routes in `internal/arm/router.go`.
3. Write unit tests in `internal/arm/{resource}_test.go` using the helpers
   from `internal/arm/testutil_test.go`.
4. Add an integration test case in `internal/arm/integration_test.go`.
5. Add a Terraform example in `examples/terraform/`.
6. Update [`docs/PARITY.md`](docs/PARITY.md) with a `Full` row and a link
   to the test that proves it.
7. Add a changelog entry under `[Unreleased]` in
   [`CHANGELOG.md`](CHANGELOG.md).

## Test requirements

Every PR must pass `make test`. Coverage targets per package are defined in
[`.claude/rules/tests.md`](.claude/rules/tests.md). The highlights:

- `internal/arm/` -- table-driven tests for each CRUD verb, error paths
  (missing api-version, 404, duplicate PUT), and at least one integration
  test case per resource.
- `internal/metadata/` -- pin the canonical metadata response shape.
- `internal/auth/` -- token fields, TLS cert generation.
- `internal/middleware/` -- path normalization, api-version enforcement.

Run `go test -race -coverprofile=coverage.out ./...` locally to check
coverage before pushing.

## PR checklist

Before opening a pull request, confirm:

- [ ] `make test` passes locally (or `go test ./... -v -count=1`).
- [ ] `pre-commit run --all-files` passes.
- [ ] Tests are added or updated for the change.
- [ ] `docs/PARITY.md` is updated if the change adds or modifies a resource.
- [ ] `CHANGELOG.md` has an entry under `[Unreleased]`.
- [ ] No new dependencies added without approval.
- [ ] Commit messages are one logical change each.

## Code review

All PRs require at least one review before merging. Reviewers check:

1. ARM API fidelity (response shapes, error format, headers).
2. Test coverage (unit + integration for resource changes).
3. Go conventions (`docs/CONVENTIONS.md` section 1).
4. Documentation drift (parity matrix, changelog, README if applicable).

## Questions?

Open an issue or start a discussion. For security vulnerabilities, see
[`SECURITY.md`](SECURITY.md).
