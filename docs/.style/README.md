# `docs/.style/`

Contributor-facing style and content guidance for the Coder documentation.
Nothing under this directory is published to
[coder.com/docs](https://coder.com/docs).

## What lives here

| Path                    | Purpose                                                             |
|-------------------------|---------------------------------------------------------------------|
| `content-guidelines.md` | Canonical content rules: what belongs in `docs/`, what doesn't, why |
| `style-guide/`          | Canonical prose style guide for `docs/`                             |
| `styles/Coder/`         | Custom Vale rules specific to Coder (product voice, terms)          |

See [`content-guidelines.md`](content-guidelines.md) for the canonical
rules on what content belongs in `docs/` and what should be routed
elsewhere (blog, changelog, Support KB, etc.).

See [`style-guide/`](style-guide/README.md) for the prose style guide. The
`styles/Coder/` directory holds the custom Vale rules that enforce parts
of the guide; Vale's `StylesPath` in the repo-root `.vale.ini` points at
`docs/.style/styles/`.

## Why a hidden directory

The leading dot mirrors the `.github/`, `.vscode/`, and `.claude/`
convention already used in this repo for tooling-internal directories.
The structural Markdown linters and Vale still pick it up (except the style
guide's own prose under `style-guide/`, which is exempt); coder.com's docs
site does not. Refer to "What does not run against this directory" below for
that exemption.

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
- Vale (`make lint/prose`) lints everything here except the style guide's own
  prose under `style-guide/`: the landing page, `content-guidelines.md`, the
  annotation demo (whose `Coder.Demo*` rules are meant to fire), and the Coder
  rule docs. Refer to the repo-root `.vale.ini` for the active configuration,
  and to "What does not run against this directory" below for the one
  exemption.

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
- Vale's `Coder` rules on the style guide's own prose under
  `docs/.style/style-guide/`. The guide deliberately contains the constructs
  the rules ban, such as Don't examples and banned terms named in headings and
  prose, so the repo-root `.vale.ini` clears `BasedOnStyles` for
  `docs/.style/style-guide/**`. Nothing else under `docs/.style/` is exempt:
  the landing page, `content-guidelines.md`, the annotation demo, and the Coder
  rule docs are all linted. Zero-baseline is therefore measured over `docs/`
  excluding `docs/.style/style-guide/`.
  The exemption applies only when the path resolves to
  `docs/.style/style-guide/...` from the working directory, which is what
  `make lint/prose` and CI pass (repo-root-relative `docs/...`). Other
  invocation forms bypass the glob and still flag the guide's intentional
  examples: absolute paths (what editors and LSP integrations pass) and
  subdirectory-relative paths (for example `vale .style/style-guide/...` run
  from `docs/`). Treat those in-editor and subdirectory findings on the guide
  as expected.

## Editing the content guidelines

Open a PR against `docs/.style/content-guidelines.md`. The rules in that
file apply to humans and AI-assisted workflows alike; when it conflicts
with another style or contributing doc in the repo, it governs.

## Editing the style guide

Open a PR against the appropriate subpage of `docs/.style/style-guide/`.
Follow-up PRs add each rule and the matching style-guide section together.

## Adding a Vale rule

Each rule in the repo-root `.vale.ini` ships clean: zero baseline
findings against the current `docs/` corpus, excluding
`docs/.style/style-guide/`, which is exempt because the style guide
demonstrates the violations the rules ban.
The PR that adds a rule is the rule's complete unit:

1. **Cleanup commit**: fix every existing-content violation of the new
   rule so `make lint/prose` reports zero findings for it.
   The cleanup ships in the same PR as the enable, ordered first.
2. **Enable commit**: add the rule to `.vale.ini` at its chosen severity, write a corresponding section under the matching subpage of `docs/.style/style-guide/`, and add the custom rule YAML under `docs/.style/styles/Coder/` if applicable.
   The rule's `message:` field points at the relevant style-guide subpage anchor.

Severity is a deliberate per-rule choice:

- `error` blocks merge. Use for hard policy where any violation is
  wrong: brand-name casing, first-person pronouns we ban outright,
  em-dash bans.
- `warning` surfaces an annotation without failing CI. Use for strong
  guidance with legitimate human-judgment exceptions: terms that need
  context (`disabled` as a technical state vs. ableist usage),
  judgment-bound style preferences.
- `suggestion` surfaces a `notice` annotation. Use for soft guidance
  where the right fix is contextual: noun-as-adjective patterns like
  `desired state`, wordiness, optional sentence reshaping.

The severity choice and the cleanup discipline are independent. A
rule landing at `warning` or `suggestion` still ships with zero
baseline findings; the rule's purpose is to catch new violations, not
to surface a backlog of existing ones. A rule that surfaces a standing
backlog teaches contributors to ignore the annotation channel, which
erodes trust in CI regardless of the severity at which the noise
arrives.

PR title: `feat(docs/.style): enable <RuleName>`.

False-positive policy: one confirmed false positive after enable,
either refine the rule or revert.
We do not maintain rules that occasionally cry wolf, regardless of
severity.

If a policy is judgment-bound (passive voice, weasel words,
sentence-case headings on a corpus with many proper nouns), write a
Coder-authored rule with the precision the situation needs instead of
accepting an imprecise third-party rule at any severity.

This applies equally to Coder-authored rules (under
`docs/.style/styles/Coder/`) and third-party rules from Google, alex,
and write-good.
Third-party rules are not loaded by default.
Each returns through the same per-rule PR pattern after its corpus is
clean.
