# gh-pages Documentation Site

Date: 2026-04-25
Status: Draft
Goal: Ship a minimum viable documentation site on GitHub Pages that makes
azemu look like a serious, well-maintained project worth starring and
contributing to.

## Decisions

- **Static site generator:** MkDocs with Material for MkDocs theme.
- **Hosting:** GitHub Pages at `zerodeth.github.io/azemu`. Custom domain
  deferred; transition is a one-line `site_url` + CNAME change.
- **Visual design:** Deferred to a separate frontend-designer skill session.
  This spec ships with Material defaults only.
- **Content strategy:** Organize around the developer journey, not tool
  integrations. Three genuinely new pages; the rest adapt existing markdown.
- **Sensitive files:** `TASKS.md`, `TODO.md`, `CLAUDE.md`, `AGENTS.md`,
  `.claude/`, `docs/ORCHESTRATION.md`, `docs/CONVENTIONS.md`,
  `docs/CHECKLISTS.md` are never published to the site.

## Directory Layout

```text
website/
  mkdocs.yml
  docs/
    index.md
    getting-started/
      install.md
      first-apply.md
      how-it-works.md
    concepts/
      architecture.md
      arm-fidelity.md
      parity-matrix.md
    resources/
      troubleshooting.md
      design-decisions/
        0001-delegate-storage-data-plane-to-azurite.md
    community/
      contributing.md
      roadmap.md
      code-of-conduct.md
    reference/
      setup.md
      changelog.md
  overrides/            # Material theme overrides (future)
```

Source lives on `main` in `website/`. The existing `docs/` at repo root
stays untouched for in-repo contributor reference.

## GitHub Actions Workflow

File: `.github/workflows/docs.yml`

- Triggers on push to `main` when `website/**` or key docs files change.
- Runs `mkdocs gh-deploy --force` to build and push to `gh-pages` branch.
- Uses `actions/setup-python` + `pip install mkdocs-material`.
- Site URL: `zerodeth.github.io/azemu`.

## MkDocs Configuration

File: `website/mkdocs.yml`

```yaml
site_name: azemu
site_url: https://zerodeth.github.io/azemu
site_description: A local Azure emulator for Terraform-first development
repo_url: https://github.com/zerodeth/azemu
repo_name: zerodeth/azemu

theme:
  name: material
  features:
    - navigation.sections
    - navigation.expand
    - search.suggest
    - search.highlight
    - content.code.copy
  palette:
    - scheme: default
      toggle:
        icon: material/brightness-7
        name: Switch to dark mode
    - scheme: slate
      toggle:
        icon: material/brightness-4
        name: Switch to light mode

nav:
  - Home: index.md
  - Getting Started:
      - Install: getting-started/install.md
      - Your First Apply: getting-started/first-apply.md
      - How It Works: getting-started/how-it-works.md
  - Concepts:
      - Architecture: concepts/architecture.md
      - ARM Fidelity: concepts/arm-fidelity.md
      - Parity Matrix: concepts/parity-matrix.md
  - Resources:
      - Troubleshooting: resources/troubleshooting.md
      - Design Decisions:
          - resources/design-decisions/0001-delegate-storage-data-plane-to-azurite.md
  - Community:
      - Contributing: community/contributing.md
      - Roadmap: community/roadmap.md
      - Code of Conduct: community/code-of-conduct.md
  - Reference:
      - Setup (Dev Env): reference/setup.md
      - Changelog: reference/changelog.md

markdown_extensions:
  - admonition
  - pymdownx.details
  - pymdownx.superfences
  - pymdownx.tabbed:
      alternate_style: true
  - pymdownx.highlight:
      anchor_linenums: true
  - pymdownx.inlinehilite
  - toc:
      permalink: true
```

## Content Plan

### New pages (need writing)

1. **Home (`index.md`):** One-liner description, three value propositions
   (no subscription, no provider forks, Terraform-first), 5-line quickstart
   code block, link to "Your First Apply."

2. **Your First Apply (`getting-started/first-apply.md`):** Step-by-step
   walkthrough of `docker compose up`, `terraform init`, `apply`, `destroy`
   with expected output at each step.

3. **ARM Fidelity (`concepts/arm-fidelity.md`):** What "emulate" means in
   azemu's context: idempotent PUT, async DELETE, HEAD semantics, Azure
   error shapes. Extracted from `docs/CONVENTIONS.md` S2 but rewritten for
   a non-contributor audience. This is azemu's technical differentiator.

### Adapted pages (light edit from existing)

| Page | Source | Edit scope |
|------|--------|------------|
| Install | README.md Docker + flox sections | Extract, rewrite as standalone |
| How It Works | docs/ARCHITECTURE.md | Simplify for non-contributor audience |
| Architecture | docs/ARCHITECTURE.md | Remove internal implementation detail |

### Copy-as-is pages

| Page | Source |
|------|--------|
| Parity Matrix | docs/PARITY.md |
| Troubleshooting | docs/TROUBLESHOOTING.md |
| Design Decisions | docs/adr/*.md |
| Contributing | CONTRIBUTING.md |
| Roadmap | ROADMAP.md |
| Code of Conduct | CODE_OF_CONDUCT.md |
| Setup (Dev Env) | docs/SETUP.md |
| Changelog | CHANGELOG.md |

### Excluded from site (internal only)

- `TASKS.md` -- implementation plan
- `TODO.md` -- known gaps and post-mortems
- `CLAUDE.md` -- Claude Code steering
- `AGENTS.md` -- agent configuration
- `.claude/` -- agent/skill definitions
- `docs/ORCHESTRATION.md` -- multi-agent patterns
- `docs/CONVENTIONS.md` -- internal coding conventions
- `docs/CHECKLISTS.md` -- internal checklists

## Future Work (not in this spec)

- Visual design customization via frontend-designer skill
- Custom domain registration and CNAME setup
- SEO metadata, OpenGraph images for social sharing
- GitHub Sponsors integration
- Blog/announcements section
- Contributor hiring and licensing strategy
