# Checklists -- azemu

Recipes for common multi-file changes. These are reference, not steering —
they do not load into Claude Code's context on every session.

## Add a new ARM resource type

1. Create `internal/arm/{resource}.go` with CRUD + HEAD handlers. Follow the
   pattern in `internal/arm/vnet.go` and `internal/arm/subnet.go`.
2. Register routes in `internal/arm/router.go` using lowercase chi literals.
3. Add shared types to `pkg/armtypes/` if needed.
4. Write unit tests: `internal/arm/{resource}_test.go` using `newTestServer(t)`
   from `testutil_test.go`. Cover:
   - [ ] PUT returns 201 on create, 200 on update
   - [ ] GET returns 200 with correct shape, 404 when missing
   - [ ] DELETE returns 202, subsequent GET returns 404
   - [ ] HEAD returns 204/404 (no body)
   - [ ] LIST returns `{"value": [...]}` wrapper
   - [ ] Missing api-version returns 400
   - [ ] Invalid body returns 400 with Azure error format
   - [ ] For child resources: PUT returns 404 `ParentResourceNotFound` when
         parent does not exist
5. Add integration test case in `test/integration/arm_test.go`.
6. Add Terraform example in `test/terraform/main.tf`.
7. Update `docs/PARITY.md` with Full/Stub/None status.
8. Update `README.md` support table.
9. Run the full validation sequence (below).

## Modify the store interface

1. Update `internal/store/store.go` interface.
2. Update `internal/store/memory.go` implementation.
3. Update `internal/store/file.go` if it exists.
4. Update all store tests.
5. Verify no `internal/arm/` tests break.
6. Run the full validation sequence.

## Before any commit

- [ ] Working on a feature branch (the `no-commit-to-branch` pre-commit hook
      blocks commits to `main`)
- [ ] `go vet ./...` passes
- [ ] `go test ./... -count=1 -race` passes
- [ ] `go build -o bin/azemu ./cmd/azemu` succeeds
- [ ] `pre-commit run --files <changed-files>` passes (auto-installed by
      `flox activate`)
- [ ] No new dependencies in `go.mod` / `go.sum` without approval
- [ ] `wc -l CLAUDE.md AGENTS.md` still under the per-session steering
      budget (Anthropic guidance: CLAUDE.md ≤200 lines)
- [ ] `docs/PARITY.md` updated if resource handlers changed
- [ ] No TODO without a tracking entry in `TODO.md`

## Full validation sequence

Run all of these before submitting a PR:

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

## End-to-end Terraform validation

For changes that affect the ARM surface, the metadata service, or the
middleware stack, run the real provider through a full apply+destroy cycle:

```bash
# Inside flox:
azemu-start        # builds, starts, prints one-time cert trust command
ta                 # terraform apply -auto-approve
td                 # terraform destroy -auto-approve
azemu-stop
```

Check `curl -sk https://127.0.0.1:4566/api/unhandled` after the run. An empty
list (`{"unhandled_routes":null}`) is green. Any entries are new gaps and
should be recorded in `TODO.md`.
