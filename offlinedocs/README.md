# Offline docs

This app builds the offline documentation bundle, `build/coder_docs_<version>.tgz`, that each release attaches for air-gapped installs.
It is a thin consumer of [coder/docs-engine](https://github.com/coder/docs-engine): the app files here are the engine's offline export kit, and the rendering, theme and transforms come from the `@coder/*` packages that each docs-engine release publishes as GitHub Release assets.

## Building

From the repository root:

```sh
make build/coder_docs_"$(./scripts/version.sh)".tgz
```

The target installs the app, syncs `docs/` into it, runs `next build`, and packs `out/`.
To run the steps yourself:

```sh
cd offlinedocs
pnpm install --frozen-lockfile
DOCS_VERSION=v2.39.2 pnpm build
```

Installs are always frozen: `pnpm-lock.yaml` records each engine tarball's integrity hash, so a release asset that changed after the lockfile was written fails the install.

- `DOCS_ROOT` is the `docs/` tree to build. It defaults to this repository's `docs/`.
- `DOCS_VERSION` is the version the export names itself: `v2.39.2`, `v2.39` or `main` (the default).
  The Makefile derives it from `scripts/version.sh` with `scripts/docs-version.mjs`: a release names itself, a release candidate names its minor line, and a development build is `main`.

## Toolchain

The app needs Node 22 or later and pnpm 10.33.2, the versions in `package.json`'s `engines` and `packageManager`.
pnpm 12 switches itself to the pinned version when it runs here.
The Makefile runs `scripts/check-toolchain.mjs` before installing or building, and fails with the versions it found.

## Updating the engine

The engine release lives in one place, `ENGINE_RELEASE_TAG` in `scripts/engine-release.mjs`.
To move to a new release:

1. Set `ENGINE_RELEASE_TAG` to the new tag, such as `v0.2.0`.
2. Run `pnpm engine:update`, which rewrites the `@coder/*` dependencies in `package.json`.
3. Run `pnpm install --lockfile-only` and commit `package.json` and `pnpm-lock.yaml`.
4. Compare the app files with the release's kit, `coder-docs-builder-<version>.tgz`, and copy over any that changed.

`pnpm lint` fails when the dependencies and the tag disagree.

## Checks

- `pnpm format:check` checks formatting with oxfmt, docs-engine's formatter.
- `pnpm test` runs the unit tests for the scripts in `scripts/`.
- `pnpm lint` checks the engine dependencies, then type-checks the app against a fresh sync of `docs/`.
