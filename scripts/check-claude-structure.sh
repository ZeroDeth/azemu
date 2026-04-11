#!/usr/bin/env bash
#
# Claude Code steering structure drift guard.
#
# Runs via .pre-commit-config.yaml on every commit. Blocks commits that
# would silently regress the structure documented in CLAUDE.md, AGENTS.md,
# and the Anthropic memory / skills / subagents guides.
#
# Checks performed:
#
#   1. CLAUDE.md stays under the Anthropic 200-line target.
#      Source: https://code.claude.com/docs/en/memory
#              "Size: target under 200 lines per CLAUDE.md file. Longer
#               files consume more context and reduce adherence."
#
#   2. No machine-local path references (~/.claude, /Users/foo,
#      .claude/projects/..., feedback_claude_md_steering) in any tracked
#      file except CHANGELOG.md and this script itself. CHANGELOG is
#      excluded because keep-a-changelog entries legitimately describe
#      past state including paths that have since moved.
#      Rationale: Anthropic's memory docs state auto memory is
#      machine-local and not shared across machines, so any committed
#      reference to it breaks for every other contributor.
#
#   3. Every .claude/agents/*.md and .claude/skills/*/SKILL.md has a
#      'name:' field in frontmatter. Without it, Claude Code does not
#      discover the file.
#
#   4. .gitignore keeps the .claude/rules/, .claude/agents/, and
#      .claude/skills/ negations. The bare .claude/* glob would hide
#      all three directories by default.
#
# Bypass for a genuine emergency (do not make a habit of this):
#
#   git commit --no-verify
#
# Manual invocation outside pre-commit:
#
#   scripts/check-claude-structure.sh

set -euo pipefail

# Run from the repo root regardless of where the hook invokes the script.
cd "$(git rev-parse --show-toplevel)"

fail=0

if [ -t 1 ]; then
  red=$'\033[0;31m'
  green=$'\033[0;32m'
  reset=$'\033[0m'
else
  red=""
  green=""
  reset=""
fi

fail_msg() {
  printf "%sFAIL%s: %s\n" "$red" "$reset" "$1" >&2
  fail=1
}

ok_msg() {
  printf "%sOK  %s: %s\n" "$green" "$reset" "$1"
}

# ---------------------------------------------------------------------------
# 1. CLAUDE.md budget
# ---------------------------------------------------------------------------
if [ -f CLAUDE.md ]; then
  lines=$(wc -l < CLAUDE.md | tr -d ' ')
  if [ "$lines" -gt 200 ]; then
    fail_msg "CLAUDE.md is $lines lines, must stay <=200"
    printf "       Anthropic target: https://code.claude.com/docs/en/memory\n" >&2
    printf "       Fix: move content to docs/ or .claude/rules/ (path-scoped)\n" >&2
    printf "       or .claude/skills/ (on-demand), then @-import if needed.\n" >&2
  else
    ok_msg "CLAUDE.md is $lines lines (target <=200)"
  fi
else
  fail_msg "CLAUDE.md is missing"
fi

# ---------------------------------------------------------------------------
# 2. No machine-local path leaks
# ---------------------------------------------------------------------------
# The pattern catches:
#   ~/.claude       - literal home-dir reference
#   /Users/foo      - macOS absolute path to a user home
#   /.claude/projects - Claude Code auto-memory directory (any mention)
#   feedback_claude_md_steering - the specific broken memory filename
#                                 from a 2026-04-11 incident
leak_pattern='(~/\.claude|/Users/[a-zA-Z]|/\.claude/projects|feedback_claude_md_steering)'
leak_excludes='^(CHANGELOG\.md|scripts/check-claude-structure\.sh)$'

tracked=$(git ls-files | grep -Ev "$leak_excludes" || true)
leaks=""
if [ -n "$tracked" ]; then
  leaks=$(echo "$tracked" | xargs grep -lE "$leak_pattern" 2>/dev/null || true)
fi

if [ -n "$leaks" ]; then
  fail_msg "machine-local path references found in:"
  echo "$leaks" | sed 's/^/       /' >&2
  printf "       These paths only exist on one machine and break for every\n" >&2
  printf "       other contributor. Inline the content or link to a public URL.\n" >&2
else
  ok_msg "no machine-local path leaks in tracked files"
fi

# ---------------------------------------------------------------------------
# 3. Agent and skill frontmatter
# ---------------------------------------------------------------------------
frontmatter_fail=0
agent_count=0
skill_count=0

for f in .claude/agents/*.md; do
  [ -f "$f" ] || continue
  agent_count=$((agent_count + 1))
  if ! head -10 "$f" | grep -qE '^name:'; then
    fail_msg "$f missing 'name:' frontmatter"
    frontmatter_fail=1
  fi
done

for f in .claude/skills/*/SKILL.md; do
  [ -f "$f" ] || continue
  skill_count=$((skill_count + 1))
  if ! head -10 "$f" | grep -qE '^name:'; then
    fail_msg "$f missing 'name:' frontmatter"
    frontmatter_fail=1
  fi
done

if [ "$frontmatter_fail" -eq 0 ]; then
  ok_msg "$agent_count agents and $skill_count skills have valid frontmatter"
fi

# ---------------------------------------------------------------------------
# 4. .gitignore negations
# ---------------------------------------------------------------------------
gitignore_fail=0
for dir in rules agents skills; do
  if ! grep -qE "^!\.claude/$dir/?$" .gitignore 2>/dev/null; then
    fail_msg ".gitignore is missing '!.claude/$dir/' negation"
    printf "       Without it, the .claude/* glob hides .claude/%s/ entirely.\n" "$dir" >&2
    gitignore_fail=1
  fi
done

if [ "$gitignore_fail" -eq 0 ]; then
  ok_msg ".gitignore preserves .claude/rules/, .claude/agents/, .claude/skills/"
fi

# ---------------------------------------------------------------------------
exit "$fail"
