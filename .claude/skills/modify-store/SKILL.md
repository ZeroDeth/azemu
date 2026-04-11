---
name: modify-store
description: Modify the in-memory store interface in internal/store and propagate the change through every implementation, every ARM handler, and every test. Use when adding a new store method, changing an existing signature, or preparing for the file-backed store in Phase 4.
---

# Modify the store interface

The store interface is load-bearing for every ARM handler. Changing it
is a careful, repo-wide operation. Follow these steps in order.

## 1. Update the interface

Edit `internal/store/store.go` to add, remove, or change the method.
Document the new contract in a comment above the method.

## 2. Update every implementation

- `internal/store/memory.go` (the in-memory implementation that ships
  today).
- `internal/store/file.go` (the file-backed implementation; only
  exists once Phase 4 lands. If it does not exist yet, skip but
  verify later).

Each implementation must honour the new contract exactly.

## 3. Update ARM handlers

Every ARM handler in `internal/arm/*.go` uses the store. Audit:

```bash
grep -rn "a.store\." internal/arm/
```

Fix every call site that is affected by the interface change. Watch
out for the `_ = a.store.Put(id, res)` pattern: today `Put` cannot
fail, but the file-backed store will. When the file store lands, every
ignored Put becomes a silent data loss (documented in TODO.md Known
Gaps).

## 4. Update tests

- Store unit tests in `internal/store/*_test.go`.
- ARM handler tests in `internal/arm/*_test.go` (the shared
  `newTestServer(t)` builds an in-memory store, so most handler tests
  keep working unchanged unless the interface change affects the
  fixture).

## 5. Run the full test suite

```bash
go vet ./...
go test ./... -v -count=1 -race
go build -o bin/azemu ./cmd/azemu
```

If a handler test breaks because the store's behaviour changed, fix
the handler, not the test.

## 6. Update docs

- `docs/CONVENTIONS.md` S4 (testing strategy) if the change affects
  how tests construct a store.
- `TODO.md` Known Gaps entry about `store.Put` if the change affects
  it.

## 7. Before committing

Run the `before-commit` skill (`/before-commit`).

## When to escalate

If the change requires touching `internal/auth`, `internal/metadata`,
or `internal/middleware`, stop and ask the caller first. Those
packages are standalone and are not supposed to depend on the store.
