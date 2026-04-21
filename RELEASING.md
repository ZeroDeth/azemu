# Releasing azemu

Checklist for cutting a new release. Follow in order.

## 1. Pre-release checks

- [ ] `make test` passes locally.
- [ ] `pre-commit run --all-files` is clean.
- [ ] `CHANGELOG.md` has an `[Unreleased]` section with all changes since
      the last tag.
- [ ] `docs/PARITY.md` is up to date (no `Full` row without a Proof link).
- [ ] `README.md` roadmap checklist reflects current state.

## 2. Prepare the changelog

- Rename `[Unreleased]` to `[vX.Y.Z] - YYYY-MM-DD`.
- Add a new empty `[Unreleased]` section above it.
- Commit: `Prepare CHANGELOG for vX.Y.Z`.

## 3. Tag

```bash
git tag -a vX.Y.Z -m "vX.Y.Z"
git push origin vX.Y.Z
```

## 4. Build and publish

Run goreleaser (once `.goreleaser.yml` is set up, task 5.9):

```bash
goreleaser release --clean
```

This produces:

- macOS, Linux, and Windows binaries.
- Docker image pushed to `ghcr.io/zerodeth/azemu:vX.Y.Z`.

## 5. GitHub release

goreleaser creates the GitHub release automatically. Review the
auto-generated notes and edit if needed. Link to the changelog section
for the full list.

## 6. Post-release

- [ ] Verify the Docker image: `docker pull ghcr.io/zerodeth/azemu:vX.Y.Z`.
- [ ] Verify the binary: download from the release, run `azemu --version`.
- [ ] Announce in the repo discussions (once community grows).
