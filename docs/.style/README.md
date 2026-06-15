# `docs/.style/`

Contributor-facing style and content guidance for the Coder documentation.
Nothing under this directory is published to
[coder.com/docs](https://coder.com/docs).

## What lives here

| Path                    | Purpose                                                             |
|-------------------------|---------------------------------------------------------------------|
| `content-guidelines.md` | Canonical content rules: what belongs in `docs/`, what doesn't, why |

See [`content-guidelines.md`](content-guidelines.md) for the canonical
rules on what content belongs in `docs/` and what should be routed
elsewhere (blog, changelog, Support KB, etc.).

> [!NOTE]
> This directory is the home for the docs scaffold being built out under
> DOCS-180. The prose style guide and the Vale rules that enforce it land
> in that work and will appear in this table when they merge.

## Why a hidden directory

The leading dot mirrors the `.github/`, `.vscode/`, and `.claude/`
convention already used in this repo for tooling-internal directories.
The structural Markdown linters still pick it up; coder.com's docs site
does not.

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
  explicitly skips paths that are not in the manifest.

Net result: not adding anything from `docs/.style/` to `docs/manifest.json`
gives us no route, no Algolia record, and no sidebar entry.

## What still runs against this directory

- `make lint/markdown` (markdownlint-cli2) processes every Markdown file
  here. The repo-root `package.json` invokes
  `markdownlint-cli2 --fix $(find docs -name '*.md')`.
- `make fmt/markdown` (markdown-table-formatter) reflows tables here for
  the same reason.

## Editing the content guidelines

Open a PR against `docs/.style/content-guidelines.md`. The rules in that
file apply to humans and AI-assisted workflows alike; when it conflicts
with another style or contributing doc in the repo, it governs.
