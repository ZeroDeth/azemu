# Checklists -- azemu

The contents of this file have moved into invocable skills under
`.claude/skills/`. Each skill carries the same checklist content, plus
frontmatter that makes it discoverable via the slash menu and (where
applicable) delegatable to a subagent.

| Old section | Now lives at | Invoke as |
|-------------|--------------|-----------|
| Add a new ARM resource type | `.claude/skills/add-resource/SKILL.md` | `/add-resource` |
| Modify the store interface | `.claude/skills/modify-store/SKILL.md` | `/modify-store` |
| Before any commit + Full validation sequence | `.claude/skills/before-commit/SKILL.md` | `/before-commit` |
| End-to-end Terraform validation | `.claude/skills/validate-terraform/SKILL.md` | `/validate-terraform` |

Subagent role definitions (arm-resource-implementer, test-writer,
code-reviewer, terraform-compatibility-debugger, docs-writer) live in
`.claude/agents/*.md`. Multi-agent composition patterns live in
`docs/ORCHESTRATION.md`.
