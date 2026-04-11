# Orchestration -- azemu

Patterns for composing multiple subagents on a single piece of work.
Individual subagent role definitions live in `.claude/agents/*.md` as
frontmatter-driven files; this document describes how to combine them.

Subagents inherit the rules from `CLAUDE.md`, `AGENTS.md`, and any
`.claude/rules/*.md` that match the files they are editing. They do not
inherit skills; if a subagent needs a skill, preload it via the skill's
name or reference the skill file path in the subagent's prompt.

## Pattern: Parallel resource implementation

When implementing multiple independent ARM resources (e.g., DNS zones
and storage accounts), fan out two `arm-resource-implementer` subagents
and merge their output.

```text
Main agent:
  1. Define shared types in pkg/armtypes/ if needed.
  2. Update internal/arm/router.go with route stubs (lowercase chi literals).
  3. Spawn subagents in parallel:

     Subagent A: arm-resource-implementer for DNS Zone
     Subagent B: arm-resource-implementer for Storage Account

  4. After both return, run the code-reviewer subagent on the merged diff.
  5. Run the full test suite and the validate-terraform skill.
  6. Update PARITY.md and README.md via docs-writer.
```

VNets + Subnets shipped via this pattern on `feat/vnet-subnet`. That
pair is a good reference for parent/child resources where the child
must validate the parent's existence on PUT.

## Pattern: Test-then-fix

When fixing Terraform compatibility issues, run the debugger and the
test writer in parallel: the debugger finds the root cause while the
test writer produces a failing regression test. Then the main agent
implements the fix and verifies it against the pre-written test.

```text
Main agent:
  1. Run terraform apply, capture the full error output and azemu logs.
  2. Spawn subagents in parallel:

     Subagent A: terraform-compatibility-debugger
       Input: error output + logs
       Output: root-cause analysis + minimal-fix proposal

     Subagent B: test-writer
       Input: expected behaviour (the behaviour the provider wants)
       Output: a failing test that pins the expected behaviour

  3. Apply the fix from subagent A.
  4. Run the failing test from subagent B; it must now pass.
  5. Run the validate-terraform skill to confirm the apply loop succeeds.
```

## Pattern: Test coverage push

When filling coverage gaps across multiple packages (Phase 2 in
`TASKS.md`), fan out `test-writer` subagents, one per package, up to
three in parallel.

```text
Main agent:
  1. Run the coverage report:
     go test ./... -coverprofile=c.out && go tool cover -func=c.out
  2. Identify packages below the per-package target in
     .claude/rules/tests.md.
  3. Spawn subagents (max 3 parallel):

     Subagent A: test-writer for internal/store
     Subagent B: test-writer for internal/auth
     Subagent C: test-writer for internal/middleware

  4. After all subagents return:
     - Run the full suite with the race detector.
     - Verify per-package coverage targets.
     - Run the code-reviewer subagent on the aggregate diff.
```

## When NOT to orchestrate

A single subagent is usually enough. Fan out only when:

- The work splits cleanly along file boundaries (each subagent owns a
  disjoint set of files).
- The subagents do not need to see each other's output mid-task.
- Merging their output back is cheap (a single code-reviewer pass is
  sufficient).

If any of those fail, do the work in a single session instead. Agent
teams (independent Claude Code sessions with peer-to-peer messaging)
are the next step up if even a single fan-out is not enough, but azemu
is not there yet.
