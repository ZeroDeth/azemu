---
paths:
  - docs/PARITY.md
  - docs/TROUBLESHOOTING.md
  - docs/SETUP.md
  - docs/ARCHITECTURE.md
  - docs/adr/**
  - CONTRIBUTING.md
  - ROADMAP.md
  - CODE_OF_CONDUCT.md
  - CHANGELOG.md
---

# Website sync rule

The project publishes a documentation site at zerodeth.github.io/azemu
from the `website/` directory. The site is built by MkDocs and deployed
via GitHub Actions on push to `main`.

When you modify any of the source files listed in the `paths` above,
check whether the corresponding page in `website/docs/` also needs
updating. The mapping is:

| Source file | Website page |
|-------------|-------------|
| docs/PARITY.md | website/docs/concepts/parity-matrix.md |
| docs/TROUBLESHOOTING.md | website/docs/resources/troubleshooting.md |
| docs/SETUP.md | website/docs/reference/setup.md |
| docs/ARCHITECTURE.md | website/docs/concepts/architecture.md, website/docs/getting-started/how-it-works.md |
| docs/adr/*.md | website/docs/resources/design-decisions/*.md |
| CONTRIBUTING.md | website/docs/community/contributing.md |
| ROADMAP.md | website/docs/community/roadmap.md |
| CODE_OF_CONDUCT.md | website/docs/community/code-of-conduct.md |
| CHANGELOG.md | website/docs/reference/changelog.md |

Copy-as-is pages (Parity Matrix, Troubleshooting, Setup, Changelog,
Code of Conduct) can be updated by copying the source file content.
Adapted pages (Architecture, How It Works, Contributing, Roadmap) need
manual review because they have been edited for the public audience.

Do NOT publish these files to the website:

- TASKS.md, TODO.md, CLAUDE.md, AGENTS.md
- .claude/ directory
- docs/ORCHESTRATION.md, docs/CONVENTIONS.md, docs/CHECKLISTS.md

Never push directly to the `gh-pages` branch. It is auto-generated
by the docs workflow.
