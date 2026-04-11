# AGENTS.md -- azemu

This file defines subagent skills for parallelisable work in the azemu project.
Claude Code auto-loads this file. Subagents inherit all rules from CLAUDE.md.

---

## Skill: arm-resource-implementer

Purpose: Implement a new ARM resource type end-to-end.

Input: Resource name, ARM API reference URL, Terraform resource name.

Outputs:

- `internal/arm/{resource}.go` -- CRUD handlers
- `internal/arm/{resource}_test.go` -- unit tests
- Updated `internal/arm/router.go` -- route registration
- Updated `docs/PARITY.md` -- status entry
- `test/terraform/{resource}.tf` -- Terraform example
- `test/terraform/{resource}.tftest.hcl` -- HCL test

Constraints:

- MUST follow the handler pattern in `internal/arm/router.go` (existing resource group handlers are the reference)
- MUST use `writeJSON` and `writeAzureError` helpers from `internal/arm/helpers.go`
- MUST NOT modify `internal/store/store.go` interface without escalating
- MUST NOT add new dependencies
- MUST include all test cases from the "Add a new ARM resource type" checklist in CLAUDE.md section 12
- Response shapes MUST match Azure ARM format (id, name, type, location, tags, properties)

Verification:

```bash
go test ./internal/arm/... -v -run Test{Resource}
go build -o bin/azemu ./cmd/azemu
```

---

## Skill: test-writer

Purpose: Write comprehensive tests for an existing package.

Input: Package path (e.g., `internal/auth`), current coverage gaps.

Outputs:

- `{package}/*_test.go` files
- Test helper file if shared setup is needed

Constraints:

- MUST use standard `testing` package only. No testify, no gomock.
- MUST use `httptest` for HTTP handler tests.
- MUST use table-driven tests where there are 3+ cases for the same function.
- MUST test error paths, not just happy paths.
- MUST NOT modify the code under test (test-only changes).
- Test names: `Test{Function}_{scenario}` (e.g., `TestPut_cascadeDelete`).

Verification:

```bash
go test ./{package}/... -v -count=1 -race -coverprofile=coverage.out
go tool cover -func=coverage.out
```

---

## Skill: code-reviewer

Purpose: Review a set of file changes for correctness, style, and completeness.

Input: List of changed files or a diff.

Review criteria (in priority order):

1. **Correctness**: does the code match ARM API contracts from CLAUDE.md section 6?
2. **Error handling**: no swallowed errors, proper `fmt.Errorf` wrapping with `%w`?
3. **Tests**: every new function has a test? Error paths covered?
4. **Package boundaries**: no import cycles? Dependency direction respected?
5. **Response shapes**: ARM JSON matches Azure format exactly?
6. **Logging**: structured zerolog fields, not printf-style?
7. **Style**: Go conventions, consistent naming, no unnecessary abstractions?
8. **Docs**: PARITY.md updated? README updated? TODO items tracked?

Output: Numbered list of findings. Each finding has:

- Severity: BLOCKER / WARNING / SUGGESTION
- File and line reference
- What is wrong
- How to fix it

---

## Skill: terraform-compatibility-debugger

Purpose: Diagnose why `terraform apply` fails against azemu.

Input: Terraform error output, azemu server logs.

Process:

1. Check azemu logs for unhandled routes (`GET /api/unhandled`).
2. For each unhandled route, identify what the `azurerm` provider expects.
3. Determine if the fix is:
   - A new endpoint (route it, implement handler)
   - A response shape correction (fix existing handler)
   - A missing header or status code
   - A token/auth issue
4. Propose minimal changes to make the specific `terraform apply` succeed.

Constraints:

- Fix only what is needed for the current failure. Do not speculatively add endpoints.
- Log every unhandled route encountered during debugging.
- Update TODO.md with endpoints discovered but not yet needed.

Output:

- Root cause analysis (2-3 sentences)
- Specific code changes with diffs
- Updated test to prevent regression

---

## Skill: docs-writer

Purpose: Write or update project documentation.

Input: What changed, which docs to update.

Files this skill may modify:

- `README.md`
- `docs/PARITY.md`
- `docs/ARCHITECTURE.md`
- `docs/CONTRIBUTING.md`
- `TODO.md`
- `CHANGELOG.md`

Constraints:

- No em dashes. Use commas, semicolons, or restructure.
- No AI-sounding language: avoid "leverage", "robust", "comprehensive",
  "streamline", "cutting-edge", "deep dive", "synergy", "holistic".
- Technical terms must be exact (e.g., "azurerm provider", not "Azure Terraform plugin").
- Keep README concise. Detailed docs go in `docs/`.
- Code examples in docs MUST be copy-pasteable and tested.

---

## Subagent Orchestration Patterns

### Pattern: Parallel resource implementation

When implementing multiple independent ARM resources (e.g., VNets and DNS zones):

```text
Main agent:
  1. Define shared types in pkg/armtypes/ if needed
  2. Update internal/arm/router.go with route stubs
  3. Spawn subagents:

  Subagent A (arm-resource-implementer): VNet + Subnet handlers + tests
  Subagent B (arm-resource-implementer): DNS Zone handlers + tests

  4. After both complete:
     - Review merged code for conflicts
     - Run full test suite
     - Update PARITY.md and README
```

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

### Pattern: Comprehensive test coverage push

When filling test coverage gaps across multiple packages:

```text
Main agent:
  1. Run coverage report: go test ./... -coverprofile=c.out && go tool cover -func=c.out
  2. Identify packages below target coverage
  3. Spawn subagents (max 3 parallel):

  Subagent A (test-writer): internal/store tests
  Subagent B (test-writer): internal/auth tests
  Subagent C (test-writer): internal/middleware tests

  4. After all complete:
     - Run full suite with race detector
     - Verify coverage targets met
```
