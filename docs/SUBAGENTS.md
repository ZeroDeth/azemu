# Subagents -- azemu

Role definitions and orchestration patterns for parallelisable work. These
are reference recipes, not automatically active behavior — spawn them
explicitly via the Task / Agent tool when the shape of the work matches.

Subagents inherit the rules from `AGENTS.md`, `CLAUDE.md`, and any
`.claude/rules/*.md` that match the files they are editing.

---

## Role: arm-resource-implementer

**Purpose:** Implement a new ARM resource type end-to-end.

**Input:** Resource name, ARM API reference URL, Terraform resource name.

**Outputs:**

- `internal/arm/{resource}.go` — CRUD handlers
- `internal/arm/{resource}_test.go` — unit tests
- Updated `internal/arm/router.go` — route registration
- Updated `docs/PARITY.md` — status entry
- `test/terraform/{resource}.tf` — Terraform example
- `test/terraform/{resource}.tftest.hcl` — HCL test

**Constraints:**

- Follow the handler pattern in `internal/arm/vnet.go` and
  `internal/arm/subnet.go` (the most current references; `resourcegroup.go`
  predates the shared test helpers and middleware stack).
- Use `writeJSON` and `writeAzureError` from `internal/arm/helpers.go`.
- Register routes with lowercase chi literals. `internal/middleware/pathcase.go`
  normalizes incoming camelCase paths from real azurerm clients.
- Use `newTestServer(t)` from `internal/arm/testutil_test.go` so tests run
  through the full middleware stack (`NormalizePath` → `AzureHeaders` →
  `RequireAPIVersion`).
- Do NOT modify `internal/store/store.go` interface without escalating.
- Do NOT add new dependencies.
- Include every test case from the "Add a new ARM resource type" checklist
  in `docs/CHECKLISTS.md`.
- Response shapes MUST match Azure ARM format (id, name, type, location,
  tags, properties).
- For child resources, return `404 ParentResourceNotFound` on PUT when the
  parent does not exist.

**Verification:**

```bash
go test ./internal/arm/... -v -run Test{Resource}
go build -o bin/azemu ./cmd/azemu
```

---

## Role: test-writer

**Purpose:** Write comprehensive tests for an existing package.

**Input:** Package path (e.g., `internal/auth`), current coverage gaps.

**Outputs:**

- `{package}/*_test.go` files
- Test helper file if shared setup is needed

**Constraints:**

- Standard `testing` package only. No testify, no gomock.
- Use `httptest` for HTTP handler tests.
- Use `newTestServer(t)` for ARM handler tests so the middleware stack runs.
- Table-driven tests when there are 3+ cases for the same function.
- Test error paths, not just happy paths.
- Do NOT modify the code under test.
- Test names: `Test{Function}_{scenario}` (e.g., `TestPut_cascadeDelete`).

**Verification:**

```bash
go test ./{package}/... -v -count=1 -race -coverprofile=coverage.out
go tool cover -func=coverage.out
```

---

## Role: code-reviewer

**Purpose:** Review a set of file changes for correctness, style, and
completeness.

**Input:** List of changed files or a diff.

**Review criteria (in priority order):**

1. **Correctness** — does the code match ARM API contracts in
   `docs/CONVENTIONS.md` §2?
2. **Error handling** — no swallowed errors, proper `fmt.Errorf` wrapping
   with `%w`?
3. **Tests** — every new function has a test? Error paths covered?
4. **Package boundaries** — no import cycles? Dependency direction respected?
5. **Response shapes** — ARM JSON matches Azure format exactly?
6. **Logging** — structured zerolog fields, not printf-style?
7. **Style** — Go conventions, consistent naming, no unnecessary abstractions?
8. **Docs** — `docs/PARITY.md` updated? `README.md` updated?
   `CLAUDE.md`/`AGENTS.md` NOT bloated?

**Output:** Numbered list of findings. Each finding has:

- Severity: BLOCKER / WARNING / SUGGESTION
- File and line reference
- What is wrong
- How to fix it

---

## Role: terraform-compatibility-debugger

**Purpose:** Diagnose why `terraform apply` fails against azemu.

**Input:** Terraform error output, azemu server logs.

**Process:**

1. Check azemu logs for unhandled routes (`GET /api/unhandled`).
2. For each unhandled route, identify what the `azurerm` provider expects.
3. Determine if the fix is:
   - A new endpoint (route it, implement handler).
   - A response shape correction (fix existing handler).
   - A missing header or status code.
   - A metadata schema mismatch (check `internal/metadata/service.go`
     against the canonical Azure response).
   - A case-sensitivity issue (check `NormalizePath` middleware is in the
     chain before chi matching).
4. Propose minimal changes to make the specific `terraform apply` succeed.

**Constraints:**

- Fix only what is needed for the current failure. Do NOT speculatively
  add endpoints.
- Log every unhandled route encountered during debugging.
- Update `TODO.md` with endpoints discovered but not yet needed.

**Output:**

- Root cause analysis (2-3 sentences)
- Specific code changes with diffs
- Updated test to prevent regression

---

## Role: docs-writer

**Purpose:** Write or update project documentation.

**Files this role may modify:**

- `README.md`
- `CHANGELOG.md`
- `docs/*.md`
- `TODO.md`

**Files this role must NOT modify without explicit approval:**

- `CLAUDE.md` (steering file; governed by `.claude/rules/docs.md`)
- `AGENTS.md` (primary README-for-agents; same governance)
- `.claude/rules/*.md` (path-scoped rules)

**Constraints:**

- No em-dashes. Use commas, semicolons, or restructure.
- No AI-buzzword vocabulary: avoid "leverage", "robust", "comprehensive",
  "streamline", "cutting-edge", "deep dive", "synergy", "holistic".
- Technical terms must be exact (e.g., "azurerm provider", not "Azure
  Terraform plugin").
- Keep `README.md` concise. Detailed docs go in `docs/`.
- Code examples must be copy-pasteable and tested.
- `CLAUDE.md` must stay near ≤200 lines per Anthropic guidance
  (<https://code.claude.com/docs/en/memory>). If updating steering content
  would push it past the budget, extract to `docs/` or `.claude/rules/`
  and reference instead.

---

## Orchestration patterns

### Pattern: Parallel resource implementation

When implementing multiple independent ARM resources (e.g., DNS zones and
storage accounts):

```text
Main agent:
  1. Define shared types in pkg/armtypes/ if needed
  2. Update internal/arm/router.go with route stubs (lowercase chi literals)
  3. Spawn subagents:

  Subagent A (arm-resource-implementer): DNS Zone handlers + tests
  Subagent B (arm-resource-implementer): Storage Account handlers + tests

  4. After both complete:
     - Review merged code for conflicts
     - Run full test suite
     - Update PARITY.md and README
```

VNets + Subnets shipped via this pattern in `feat/vnet-subnet`. That pair is
a good reference for parent/child resources where the child must validate the
parent's existence on PUT.

### Pattern: Test-then-fix

When fixing Terraform compatibility issues:

```text
Main agent:
  1. Run terraform apply, capture output
  2. Spawn subagents:

  Subagent A (terraform-compatibility-debugger): analyse failure
  Subagent B (test-writer): write regression test for the expected behaviour

  3. After analysis:
     - Implement fix
     - Run the pre-written test to confirm it passes
```

### Pattern: Test coverage push

When filling test coverage gaps across multiple packages:

```text
Main agent:
  1. Run coverage report:
     go test ./... -coverprofile=c.out && go tool cover -func=c.out
  2. Identify packages below target coverage (targets in docs/CONVENTIONS.md §4)
  3. Spawn subagents (max 3 parallel):

  Subagent A (test-writer): internal/store tests
  Subagent B (test-writer): internal/auth tests
  Subagent C (test-writer): internal/middleware tests

  4. After all complete:
     - Run full suite with race detector
     - Verify coverage targets met
```
