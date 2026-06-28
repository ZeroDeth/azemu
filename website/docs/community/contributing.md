# Contributing to azemu

azemu is an open-source project and contributions of every size are
welcome. You do not need to be an Azure or Go expert to help; a clear bug
report or a docs fix moves the project forward as much as a new resource
handler does.

## Ways to contribute

- **Report a compatibility gap.** If `terraform apply` or `tofu apply` fails
  against azemu, open an [issue](https://github.com/zerodeth/azemu/issues)
  with the config and the error. These are the most useful reports we get.
- **Add a resource.** The most common code contribution. See
  [Adding a new ARM resource](#adding-a-new-arm-resource) below.
- **Improve the docs.** Every page on this site has an edit pencil in the
  top-right corner that opens it directly on GitHub. Typos, clarifications,
  and new troubleshooting entries are all fair game.
- **Ask and answer questions.** Join
  [Discussions](https://github.com/zerodeth/azemu/discussions) to share what
  you are building or to help another contributor.

New contributors: look for issues labelled
[`good first issue`](https://github.com/zerodeth/azemu/labels/good%20first%20issue).
If none are open, ask in Discussions and we will help you find a starting
point.

This document covers how to add a resource, the test bar every change has to
clear, and the PR checklist. If you are new here, start with:

- The [home page](../index.md) for the project pitch and the Docker quick-start.
- [Setup Guide](../reference/setup.md) for the flox contributor workflow.
- [Architecture](../concepts/architecture.md) for the request flow and
  package layout.
- [Parity Matrix](../concepts/parity-matrix.md) for what is implemented and the
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

See [Setup Guide](../reference/setup.md) for the manual (non-flox) path, the
`AZEMU_CERT_PATH` environment variable, and the IPv6 / `localhost` gotcha.

## Adding a new ARM resource

This is the most common type of contribution. The short version:

1. Create `internal/arm/{resource}.go` with CRUD + HEAD handlers following
   the pattern in `internal/arm/vnet.go` (not `resourcegroup.go`, which
   predates the shared helpers).
2. Register routes in `internal/arm/router.go`.
3. Write unit tests in `internal/arm/{resource}_test.go` using the helpers
   from `internal/arm/testutil_test.go`.
4. Add an integration test case in `internal/arm/integration_test.go`.
5. Add a Terraform example in `examples/terraform/`.
6. Update [Parity Matrix](../concepts/parity-matrix.md) with a `Full` row and a link
   to the test that proves it.
7. Add a changelog entry under `[Unreleased]` in
   [Changelog](../reference/changelog.md).

## Test requirements

Every PR must pass `make test`. The highlights:

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
- [ ] [Parity Matrix](../concepts/parity-matrix.md) is updated if the change adds or modifies a resource.
- [ ] [Changelog](../reference/changelog.md) has an entry under `[Unreleased]`.
- [ ] No new dependencies added without approval.
- [ ] Commit messages are one logical change each.

## Code review

All PRs require at least one review before merging. Reviewers check:

1. ARM API fidelity (response shapes, error format, headers).
2. Test coverage (unit + integration for resource changes).
3. Go conventions (see the project docs for style guidance).
4. Documentation drift (parity matrix, changelog, README if applicable).

## Questions?

Open an issue or start a discussion. For security vulnerabilities, see
[`SECURITY.md`](https://github.com/zerodeth/azemu/blob/main/SECURITY.md).
