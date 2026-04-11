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
  guideline, look it up at the source before proposing alternatives. See
  memory entry `feedback_claude_md_steering.md`.

<!--
Maintainer note (stripped before context injection):
CLAUDE.md was 643 lines on 2026-04-11 and refactored to this thin wrapper
per Anthropic's <=200-line guidance. Prior sections moved to:
  S2 architecture diagram + package layout -> docs/ARCHITECTURE.md
  S3 build/test commands -> docs/SETUP.md
  S5 Go conventions -> .claude/rules/go-style.md + docs/CONVENTIONS.md S1
  S6 ARM fidelity rules -> .claude/rules/arm-handlers.md + docs/CONVENTIONS.md S2
  S7 auth fidelity rules -> docs/CONVENTIONS.md S3
  S8 testing strategy -> .claude/rules/tests.md + docs/CONVENTIONS.md S4
  S9 unhandled route strategy -> docs/CONVENTIONS.md S5
  S10 file/directory rules -> docs/CONVENTIONS.md S6
  S11 parity matrix discipline -> docs/CONVENTIONS.md S7 (+ docs/PARITY.md)
  S12 checklists -> docs/CHECKLISTS.md
The bloated version is preserved in git history. DO NOT re-bloat this file.
See memory at
~/.claude/projects/-Users-zerodeth-Projects-azemu/memory/feedback_claude_md_steering.md
for the Anthropic source quotes.
-->
