# `docs/.style/`

Contributor-facing style guide and prose-lint configuration for the Coder
documentation. Nothing under this directory is published to
[coder.com/docs](https://coder.com/docs).

## What lives here

| Path             | Purpose                                                    |
|------------------|------------------------------------------------------------|
| `style-guide.md` | Canonical prose style guide for `docs/`                    |
| `styles/Coder/`  | Custom Vale rules specific to Coder (product voice, terms) |

See [`docs/.style/style-guide.md`](style-guide.md) for the style guide
itself. The `styles/Coder/` directory holds the custom Vale rules that
enforce parts of the guide. Vale's `StylesPath` in the repo-root
`.vale.ini` points at `docs/.style/styles/`.

## Why a hidden directory

The leading dot mirrors the `.github/`, `.vscode/`, and `.claude/`
convention already used in this repo for tooling-internal directories.
Vale and the structural Markdown linters still pick it up; coder.com's
docs site does not.

## How exclusion from coder.com works

[coder.com/docs](https://coder.com/docs) routes and search are
manifest-driven:

- Route discovery lives in
  [`coder/coder.com:src/utils/docs/docs.ts`](https://github.com/coder/coder.com/blob/master/src/utils/docs/docs.ts)
  (`getDocsStaticPaths`). It iterates `routes` from `docs/manifest.json`
  and emits one Next.js static path per entry. Files not in the manifest
  do not become routes.
- The Algolia surgical indexer at
  [`coder/coder.com:src/utils/algoliaDocs/surgical.ts`](https://github.com/coder/coder.com/blob/master/src/utils/algoliaDocs/surgical.ts)
  explicitly skips paths that are not in the manifest, incrementing
  `pathsSkipped`.

Net result: not adding anything from `docs/.style/` to `docs/manifest.json`
gives us no route, no Algolia record, and no sidebar entry. Two
defense-in-depth changes in `.github/workflows/deploy-docs.yaml` keep the
deploy workflow from running on `.style`-only commits and exclude the
directory from the surgical-reindex payload on mixed commits.

## What still runs against this directory

- `make lint/markdown` (markdownlint-cli2) processes every Markdown file
  here. The repo-root `package.json` invokes
  `markdownlint-cli2 --fix $(find docs -name '*.md')`.
- `make fmt/markdown` (markdown-table-formatter) reflows tables here for
  the same reason.
- Vale, once configured per
  [DOCS-40](https://linear.app/codercom/issue/DOCS-40), lints the entire
  `docs/**/*.md` set including `docs/.style/style-guide.md`.

## What does not run against this directory

- `linkspector`: excluded via `excludedDirs` in `.github/.linkspector.yml`.
  External-link checking is overkill for contributor tooling.

## Editing the style guide

Open a PR against `docs/.style/style-guide.md`. The rule-specific tickets
in the
[Docs style guide](https://linear.app/codercom/project/docs-style-guide-7828445b9afc)
project fill in the body section by section.

## Running Vale locally

The canonical entry point is `make lint/prose`. The first run downloads
the pinned Vale binary and the configured style packages (Google, alex,
write-good) into `docs/.style/styles/`; subsequent runs are fast.

```shell
make lint/prose
```

The target wraps Vale in `|| true` so warnings do not fail the build. To
see Vale's raw exit code, invoke the binary directly:

```shell
make docs/.style/.vale-synced
./build/vale-*/vale docs/    # or pass specific files
```

`.vale.ini` at the repo root selects the curated rule set. See its
inline comments for the rationale on each enabled or disabled rule.
