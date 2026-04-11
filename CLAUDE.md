@AGENTS.md

## Claude Code overrides

The project rules live in `AGENTS.md` (imported above) and the docs it
references. This file only contains Claude-Code-specific overrides.

- **Steering budget.** Before editing `CLAUDE.md` or `AGENTS.md`, run
  `wc -l CLAUDE.md AGENTS.md` and confirm the result stays within Anthropic's
  published target of under 200 lines for `CLAUDE.md`
  (<https://code.claude.com/docs/en/memory>). New conventions go to
  `.claude/rules/*.md` with `paths:` frontmatter; new how-to content goes to
  `docs/`.
- **Path-scoped rules load conditionally.** `.claude/rules/arm-handlers.md`,
  `.claude/rules/go-style.md`, `.claude/rules/tests.md`, and
  `.claude/rules/docs.md` only load when Claude reads files matching their
  `paths:` glob. Do not duplicate their content into steering files.
- **Plan mode first.** For any change touching `internal/arm/`,
  `internal/metadata/`, `internal/middleware/`, or `internal/auth/`, propose
  a plan and wait for confirmation before editing.
- **Ask, do not guess.** When the user cites "best practice" or a published
  guideline, look it up at the source before proposing alternatives.

<!--
Maintainer note (stripped before context injection; costs zero tokens).

STEERING BUDGET RATIONALE
Anthropic guidance: target under 200 lines for CLAUDE.md. Longer files
consume more context and reduce adherence. Source (verbatim):
  "Size: target under 200 lines per CLAUDE.md file. Longer files consume
   more context and reduce adherence. If your instructions are growing
   large, split them using imports or .claude/rules/ files."
  -- https://code.claude.com/docs/en/memory

HTML comments like this one are stripped before CLAUDE.md is injected
into Claude's context, so maintainer notes cost zero session tokens.
Source (verbatim):
  "Block-level HTML comments (<!-- ... -->) in CLAUDE.md files are
   stripped before the content is injected into Claude's context. Use
   them to leave notes for human maintainers without spending context
   tokens on them."
  -- <https://code.claude.com/docs/en/memory>

REFACTOR HISTORY
CLAUDE.md was 643 lines on 2026-04-11 and refactored to this thin wrapper.
Prior sections moved to:
  architecture diagram + package layout -> docs/ARCHITECTURE.md
  build/test commands                   -> docs/SETUP.md
  Go conventions                        -> .claude/rules/go-style.md + docs/CONVENTIONS.md S1
  ARM fidelity rules                    -> .claude/rules/arm-handlers.md + docs/CONVENTIONS.md S2
  auth fidelity rules                   -> docs/CONVENTIONS.md S3
  testing strategy                      -> .claude/rules/tests.md + docs/CONVENTIONS.md S4
  unhandled route strategy              -> docs/CONVENTIONS.md S5
  file/directory rules                  -> docs/CONVENTIONS.md S6
  parity matrix discipline              -> docs/CONVENTIONS.md S7 (+ docs/PARITY.md)
  multi-step checklists and playbooks   -> .claude/skills/*/SKILL.md
  subagent role definitions             -> .claude/agents/*.md
  subagent orchestration patterns       -> docs/ORCHESTRATION.md
The bloated version is preserved in git history.
DO NOT re-bloat this file.

ASK-DO-NOT-GUESS RULE (the bullet above)
Source: 2026-04-11 feedback during the first CLAUDE.md refactor session.
Rationale: when the user cited published Anthropic guidance, Claude
proposed alternatives instead of reading the source. The rule is to look
up cited best practices at their source (link above) before proposing
anything. This is especially important because CLAUDE.md is a shared
steering file for a public project; contributors on other machines will
not have any machine-local auto-memory, so every piece of rationale must
live in the repo or at a URL that is publicly reachable.
-->
