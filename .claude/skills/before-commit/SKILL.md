---
name: before-commit
description: Run the full validation sequence before committing changes to azemu (go vet, go test with race detector, go build, integration tests, pre-commit hooks, steering-budget check). Invoke manually via /before-commit. Claude will not auto-run this skill because it takes several minutes and has side effects.
disable-model-invocation: true
---

# Before any commit

Run this skill manually before every commit. Claude will not trigger
it automatically.

## Pre-flight checklist

- [ ] Working on a feature branch. The `no-commit-to-branch` pre-commit
      hook blocks commits to `main`; if you land on `main` by accident,
      `git switch -c` to a feature branch.
- [ ] No new dependencies in `go.mod` / `go.sum` without prior approval.
- [ ] `docs/PARITY.md` updated if you touched resource handlers.
- [ ] No dangling TODO comments without a tracking entry in `TODO.md`.

## Full validation sequence

Run all of these in order. Every one must exit 0.

```bash
go vet ./...
go test ./... -v -count=1 -race
go build -o bin/azemu ./cmd/azemu
./bin/azemu &
sleep 2
go test ./test/integration/... -v -tags=integration
kill %1
pre-commit run --all-files
```

## Steering-budget check

Anthropic's published target is under 200 lines for `CLAUDE.md`.
Confirm you have not bloated it:

```bash
wc -l CLAUDE.md AGENTS.md
```

`CLAUDE.md` must be under 200 lines. `AGENTS.md` has no hard ceiling
but should stay roughly its current size.

## End-to-end Terraform (for ARM / metadata / middleware / auth changes)

If your change affects any of those packages, also run the
`validate-terraform` skill (`/validate-terraform`).

## If anything fails

- `go vet` failure: fix the vet warning. Do not silence it.
- `go test` failure: fix the broken test or the broken production
  code. Do not `t.Skip` around it.
- `go build` failure: the code does not compile; fix it.
- Integration test failure: the in-process server is rejecting
  something the unit tests did not catch. Reproduce with
  `curl -sk https://127.0.0.1:4566/api/unhandled` on a running binary.
- `pre-commit` failure: never use `--no-verify`. Fix the underlying
  issue the hook caught.
- `wc -l CLAUDE.md` over 200: move the new content to `.claude/rules/`
  (with `paths:` frontmatter) or to a file under `docs/` and reference
  it from CLAUDE.md via `@` import.

## Only then

```bash
git add <specific files>
git commit
```

Do not use `git add -A` or `git add .`; they sweep up secrets,
`.azemu/cert-bundle.pem`, editor scratch files, and other things you
did not mean to stage.
