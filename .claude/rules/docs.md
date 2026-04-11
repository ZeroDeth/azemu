---
paths:
  - "**/*.md"
---

# Documentation rules

Loaded only when Claude is reading or editing markdown files.

## Style

- No em-dashes (`—`). Use commas, semicolons, or restructure the sentence.
- No AI-buzzword vocabulary: avoid "leverage", "robust", "comprehensive",
  "streamline", "cutting-edge", "deep dive", "synergy", "holistic", "delve".
- Technical terms must be exact: "azurerm provider", not "Azure Terraform plugin".
- Code examples must be copy-pasteable. If you write one, run it before
  committing.

## Steering files (CLAUDE.md, AGENTS.md)

These files load into Claude Code's context on every session. Their combined
size is the per-session steering budget. Anthropic's published guidance:
target under 200 lines for `CLAUDE.md` alone. Source:
<https://code.claude.com/docs/en/memory>.

Before editing either file:

1. Run `wc -l CLAUDE.md AGENTS.md`.
2. Confirm the result will not bloat past the budget.
3. New conventions go to `.claude/rules/*.md` (this directory) with `paths:`
   frontmatter so they only load when matching files are touched.
4. New how-to content goes to `docs/`.
5. Multi-step procedures go to `.claude/skills/` (on-demand load).

CLAUDE.md was 643 lines on 2026-04-11 and refactored down. The bloated
version is preserved in git history if you need to retrieve content.

## Markdownlint

Pre-commit runs markdownlint with `--disable MD013 MD033 MD041`. Common
gotchas the remaining rules catch:

- **MD024 (no-duplicate-heading):** each heading must be unique within its
  parent. `CHANGELOG.md` uses an inline `<!-- markdownlint-disable MD024 -->`
  directive because keep-a-changelog reuses "### Added" / "### Changed" /
  "### Fixed" per release. Don't replicate this elsewhere; the directive is
  scoped to changelog files.
- **MD036 (no-emphasis-as-heading):** bold paragraphs are not a workaround
  for duplicate headings. Use real headings or rename one of the duplicates.
- **MD040 (fenced-code-block-language):** code blocks need a language tag
  (use `text` for non-language content like ASCII diagrams or shell output).
- **MD056 (table-column-count):** literal `|` characters inside table cells
  are counted as column separators. Escape with `\|` or rephrase.

## HTML comments

Block-level HTML comments (`<!-- ... -->`) in CLAUDE.md and AGENTS.md are
stripped before injection into Claude's context. They cost zero context
tokens. Use them freely for maintainer notes that humans should see but
Claude does not need. Comments inside fenced code blocks are preserved.

## Doc tree structure

| File | Purpose |
|------|---------|
| `README.md` | Public-facing project intro and quickstart |
| `CHANGELOG.md` | Keep-a-changelog release history |
| `TASKS.md` | Phased implementation plan, current status |
| `TODO.md` | Known gaps and post-mortems |
| `CLAUDE.md` | Steering for Claude Code (thin, imports AGENTS.md) |
| `AGENTS.md` | README for any coding agent (cross-vendor <https://agents.md> spec) |
| `docs/ARCHITECTURE.md` | Package layout, dependency graph, request flow |
| `docs/CONVENTIONS.md` | Go style, ARM contracts, auth contracts, testing strategy |
| `docs/ORCHESTRATION.md` | Multi-agent composition patterns (parallel, test-then-fix, coverage push) |
| `docs/PARITY.md` | Full/Stub/None matrix per resource |
| `docs/SETUP.md` | Contributor onboarding (flox + manual paths) |
| `docs/TROUBLESHOOTING.md` | Common errors and fixes |
| `.claude/agents/*.md` | Subagent role definitions (frontmatter-driven) |
| `.claude/skills/*/SKILL.md` | Slash-invokable playbooks |
