# `docs/.style/`

Contributor-facing style and content guidance for the Coder documentation.
Nothing under this directory is published to
[coder.com/docs](https://coder.com/docs).

## What lives here

| Path                    | Purpose                                                             |
|-------------------------|---------------------------------------------------------------------|
| `content-guidelines.md` | Canonical content rules: what belongs in `docs/`, what doesn't, why |
| `style-guide.md`        | Canonical prose style guide for `docs/`                             |
| `styles/Coder/`         | Custom Vale rules specific to Coder (product voice, terms)          |

See [`content-guidelines.md`](content-guidelines.md) for the canonical
rules on what content belongs in `docs/` and what should be routed
elsewhere (blog, changelog, Support KB, etc.).

See [`style-guide.md`](style-guide.md) for the prose style guide. The
`styles/Coder/` directory holds the custom Vale rules that enforce parts
of the guide; Vale's `StylesPath` in the repo-root `.vale.ini` points at
`docs/.style/styles/`.

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
- Vale lints the entire `docs/**/*.md` set, including
  `docs/.style/style-guide.md`. See the repo-root `.vale.ini` for the
  active configuration; run `make lint/prose` locally to reproduce.

## What does not run against this directory

- `linkspector`: excluded via `excludedDirs` in `.github/.linkspector.yml`.
  External-link checking is overkill for contributor tooling.
- The `deploy-docs` workflow: its `paths:` filter negates `docs/.style/**`,
  and the surgical-reindex git-diff invocation excludes the same path. See
  `.github/workflows/deploy-docs.yaml`.
- The `docs-preview` workflow: its `paths:` filter negates `docs/.style/**`,
  so `.style`-only PRs produce no preview comment. The selection logic also
  skips `.style` files when picking the preview target on mixed PRs. See
  `.github/workflows/docs-preview.yaml`.

## Editing the content guidelines

Open a PR against `docs/.style/content-guidelines.md`. The rules in that
file apply to humans and AI-assisted workflows alike; when it conflicts
with another style or contributing doc in the repo, it governs.

## Editing the style guide

Open a PR against `docs/.style/style-guide.md`. Follow-up PRs add each
rule and the matching style-guide section together.
