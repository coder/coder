# Coder Docs Content Guidelines

> [!NOTE]
> This is the **canonical** guidance for what belongs in the Coder
> documentation under `docs/` (published to
> [coder.com/docs](https://coder.com/docs)) and what doesn't. It applies to
> both human contributors and any LLM-assisted workflow that touches the
> docs. When this file conflicts with another style or contributing
> document in the repository, this file governs.

## How to use this guide

When you have a candidate change for the docs, apply these rules in order:

1. Walk the [quick decision checklist](#quick-decision-checklist) to triage
   the content.
2. If the checklist routes the content away from the docs, find the
   correct home in the [routing table](#routing-table).
3. If the content does belong in the docs, follow the
   [guiding principles](#guiding-principles), the
   [what belongs](#what-belongs-in-the-docs) catalog, and the
   [structural rules](#structural-rules).
4. If you're still unsure, file a question in the DOCS project in Linear or
   tag `@vigilante` on a draft PR. Don't guess.

## Quick decision checklist

Triage a piece of content fast. If any answer routes you away from the
docs, see the [routing table](#routing-table) for the correct destination.

1. Does it describe how the product works or how to use it, from the end
   user's perspective? **Likely docs.**
2. Is it announcing, celebrating, or explaining the motivation behind a
   feature? **Blog**, not docs.
3. Is it a record of what changed in a release (including performance
   improvements and bug fixes)? **Changelog**, not docs.
4. Is it about what to do when the product fails or misbehaves?
   **Support KB (Pylon)**, not docs.
5. Is it about contributing to the Coder codebase or writing style?
   **GitHub** (or public Notion), not docs.
6. Is it already documented by a third-party vendor (Terraform, AWS,
   Azure, GCP, etc.)? **Link to their docs**, don't duplicate.
7. Is it relevant only to a past or hypothetical future version? **Doesn't
   belong**; keep docs scoped to the version they describe.

## Guiding principles

### Follow the Diátaxis framework

The docs follow the [Diátaxis framework](https://diataxis.fr/). Every page
should be identifiable as one of:

- a tutorial,
- a how-to guide,
- a reference, or
- an explanation,

and should not mix those modes within a single page.

*Why:* Diátaxis gives both writers and readers a predictable structure,
and gives the team a vocabulary for detecting when a page has drifted out
of its lane.

### Describe the current version, for the end user

Every page should be accurate for the specific product version it applies
to, and oriented around what the user sees, types, and gets back.

*Why:* Most users care about direct inputs and outputs ("if I enable
setting X, I see Y"), not how Coder is implemented internally.
Version-scoped content is also what makes drift detectable and testable.

### Programmatic content is a testable CI surface

Tutorials and how-to guides that include CLI commands (or chained
commands) must state the expected output, so correctness can be verified
automatically.

*Why:* If we can run it, we can detect drift. Untestable claims rot
silently.

### Verify against the code; document exact values

Docs claims should be checked against the actual implementation, not
approximations:

- Exact RBAC action names. Example: `template:view_insights`, not "view
  insights".
- Real thresholds and defaults. Example: `green < 150ms, yellow 150-300ms,
  red ≥300ms`, not "around 150 ms".
- Full API paths. Example: `/api/v2/insights/templates`, not
  `/insights/templates`.

*Why:* Precise values are what make accuracy checkable; "roughly 5
minutes" can't drift-fail, but `300s default` can.

### Documentation lands with the change

A PR that introduces or changes a user-facing feature should include the
documentation for it, in the same PR, or land at the same time.

A feature is **user-facing** once it's visible by default: it appears in
`--help` output for a CLI command, in the UI under a section, in a public
API listing, or in a public configuration surface. A backend or API
change that's technically possible but not exposed to users by default,
including anything guarded by an unsafe experiment flag, doesn't qualify
until it's visible. See
[Experiments versus feature stages](#experiments-versus-feature-stages)
below for the experiment-vs-stage distinction.

*Why:* Docs written at PR time are written while the behavior is freshest
and are verifiable against the diff. Tying the docs bar to default
visibility keeps backend-only plumbing PRs out of the docs queue.

Three corollaries:

1. **Features that introduce or change behavior get documented in the PR
   that introduces or changes them.** Don't merge a behavior change
   without the matching doc update.
2. **Features that are not yet confirmed to exist do not get documented.**
   No speculative docs for unmerged or uncommitted work.
3. **Multi-PR launch exception.** For a body of work that spans several
   PRs and is spec'd to launch together by a particular date, docs may be
   written ahead of those merges, in the present tense, describing the
   feature as it will exist at launch. They must never read as a promise
   of what's coming. No "will support", "in a future release", "coming
   soon", or roadmap framing.

### Experiments versus feature stages

Coder has two related but distinct concepts. Don't conflate them:

- **Experiments** are the feature flagging system: the `--experiments`
  flag on `coder server` and the `CODER_EXPERIMENTS` environment
  variable. An experiment is either *safe* (ready for users to try) or
  *unsafe* (active development, not designed for users at all).
- **Feature stages** describe how production-ready a feature is: Early
  Access, Beta, or General Availability. See
  [Feature stages](../../install/releases/feature-stages.md).

Practical impact for docs:

- **Unsafe experiments** don't need docs. The feature is in active
  development, hidden behind a flag the user wouldn't enable on a real
  deployment, and may be reverted at any time.
- **Safe experiments and Early Access features** need at least a single
  docs page covering how to enable the feature, what it does, and known
  limitations.
- **Beta features** get full docs (how to use, configure, and operate),
  with the `Beta` label.
- **GA features** get full docs across reference, tutorials, and guides
  as appropriate.

*Why:* Holding unsafe-experiment PRs to the docs bar is noise. Holding
Early Access or Beta PRs to a lower bar is drift.

## What belongs in the docs

Use this catalog with the [quick decision checklist](#quick-decision-checklist)
above. Each entry includes the reason it belongs in the docs.

- **Tutorials that touch programmatic aspects of the product.** "If I run
  this group of CLI commands, what's supposed to happen?" and "How do I do
  X in the product?", each written so it can become a testable, verifiable
  CI surface.

  *Why:* A true tutorial serves the user's study and informs action; it
  teaches the right way to use a command in an approachable, no-risk way
  that a bare reference page can't.

- **Explanations of features with a direct, noticeable impact on how users
  interact with the product.**

  *Why:* If a feature changes what the user sees or does, the docs must
  explain how it's supposed to work.

  *Exception:* Performance improvements belong in the changelog or blog,
  since they don't change how the user interacts with the product.

- **Supported integrations, providers, and APIs.** Examples: the Slack
  integration, GitHub Actions, Bedrock vs. Claude as model providers.

  *Why:* Users need an authoritative answer to "does Coder work with X?",
  and this is a high-drift area worth actively monitoring.

- **New features that add genuine net-new value.** New UI sections, and
  new CLI commands or flags (e.g., key expiration policy for Coder
  secrets), including expected command flags and output.

  *Why:* Net-new surface area is undocumented by definition; documenting
  expected flags and output also feeds the testable-CI-surface goal.

- **Configuration surfaces.** New environment variables, server flags, and
  settings must be documented when they ship.

  *Why:* Configuration is product surface area just like the UI and CLI.
  If a setting changes behavior, users need an authoritative description
  of it.

- **Coder's own API endpoints.** New or changed endpoints must be
  documented with full, correct paths. Example: `/api/v2/insights/templates`,
  never `/insights/templates`.

  *Why:* The API is a first-class user surface, and imprecise paths are a
  drift vector. This is distinct from the third-party integrations rule
  above, which is about compatibility with external services.

- **Breaking changes and migration steps.** When a change breaks existing
  behavior, the docs must cover the migration path for the current
  version's upgrade.

  *Why:* Migration steps for getting onto the current version are
  current-version content. They describe what a user on this version must
  do, so they don't violate the version-scoping principle. Once a
  migration path is no longer relevant to the supported upgrade path, it
  ages out like any other stale content.

- **Tutorials and guides that go beyond an API reference.** Walkthroughs
  using the CLI (or chained commands) with expected outputs.

  *Why:* Reference docs tell users what exists; guides teach them how to
  accomplish something with it.

- **Minimal teaching examples of Terraform, with ample links to
  HashiCorp's docs.**

  *Why:* This is a deliberate exception to the "don't duplicate
  third-party docs" rule. Solutions-team experience shows many customers
  don't know how to write the Terraform needed to build workspaces that
  satisfy their business requirements. A light sprinkling of Terraform
  unblocks them; the links keep HashiCorp's docs as the source of truth.

- **Screenshots, used wisely, never reflexively.** Include a screenshot
  only when the topic would be confusing without the visual aid. The
  policy is not "no screenshots"; it is "use screenshots wisely." Every
  screenshot must follow all of these rules:

  1. No PHI or PII.
  2. No internal secrets leaked without properly obfuscating the text.
  3. Capture the minimally necessary surface area. The more area a
     screenshot includes, the more likely it becomes out of date.
  4. Alt text is always required, and must properly explain the purpose
     of the screenshot for accessibility.

  *Why:* Screenshots must be kept up to date and risk going stale if not
  actively monitored, and users who rely on screen readers or other
  assistive technology cannot get the same value from screenshots that
  sighted users can. Each screenshot must earn its place, stay small, and
  carry alt text that conveys its purpose.

  *Note:* This policy supersedes the older "image-driven documentation"
  guidance (structuring sections around screenshots, inserting
  placeholders for missing screenshots). It may be loosened if automated
  screenshot generation becomes real (see
  [Open items](#open-items)).

## Structural rules

These govern *how* content enters the docs, for both humans and the
doc-check agent.

- **Every new page must be added to `docs/manifest.json`.** Pages not in
  the manifest don't appear in navigation and effectively don't exist on
  [coder.com/docs](https://coder.com/docs).
- **Never hand-edit auto-generated content.** Files under
  `docs/reference/cli/` are generated from Go code; changes go in the CLI
  definitions (typically under `cli/`), then regenerate. Generated
  sections are marked with `<!-- Code generated ... DO NOT EDIT -->`.
- **Premium features are marked explicitly.** Both of the following are
  required for a Premium page:
  1. The H1 title takes a `(Premium)` suffix. Example: `# Template
     Insights (Premium)`.
  2. The page's `docs/manifest.json` entry gets `"state": ["premium"]`.
- **Moving or renaming a page requires link updates and a redirect.** If
  a page changes its position in the directory structure:
  1. Update every link that relies on its existing location.
  2. Add a redirect in the
     [`coder/coder.com`](https://github.com/coder/coder.com/blob/master/redirects.json)
     repo (`redirects.json`).

  Do not create a `docs/_redirects` file. That format isn't processed by
  [coder.com](https://coder.com).
- **No emdash, endash, or ` -- ` as punctuation.** This applies in docs
  prose, code blocks, comments, and string literals. Use commas,
  semicolons, or periods, or restructure the sentence. For numeric
  ranges, use a plain hyphen (e.g., `0-100`). The rule is enforced by
  `make lint/emdash`.

## What does not belong in the docs

Use this catalog alongside the [routing table](#routing-table). Each entry
includes the destination and the reason.

- **Contributing guides.** Route to GitHub directly, or possibly a public
  Notion site.

  *Why:* The number of people contributing to the Coder codebase is a
  small fraction of the number of people using the product; this content
  isn't relevant to end users.

- **Style guides (including the docs style guide).** Route to GitHub,
  alongside the code.

  *Why:* Style rules share the same logic and audience as contribution
  guidelines, and keeping docs style with the product reduces friction
  for the CI workflows that will eventually enforce it.

- **Support and troubleshooting content.** Route to the support
  knowledge base (Pylon). Troubleshooting documentation is primarily
  owned by Support, with Docs as secondary owner where needed.

  *Why:* The docs explain how the product works and how to use it; the
  KB covers what to do when things go wrong. Support should own the
  content they produce.

  *Connection point:* Docs pages should surface relevant Pylon KB
  articles via an embedded widget, scoped to the section or page. This
  keeps docs and support content separate while still giving users quick
  answers to troubleshooting questions in context. (Implementation under
  investigation.)

- **Bugs where the desired behavior isn't already documented.** Route to
  changelog.

  *Why:* The docs shouldn't highlight product deficiencies.

  *Exception:* When the behavior is bad, Coder itself agrees it's bad,
  and the docs don't yet cover what's *supposed* to happen, document the
  expected behavior and/or best practices for configuring around the
  problem.

- **Timeless, predictive, or stale content.** Only content relevant to
  the specific version a doc applies to belongs in that doc.

  *Why:* Don't predict the future; don't carry forward material that no
  longer applies. Version-scoped content is what keeps the docs
  trustworthy and drift-detectable. See the multi-PR launch exception in
  [Documentation lands with the change](#documentation-lands-with-the-change).

- **Feature announcements and launch rationale.** Route to blog.

  *Why:* Announcing a feature, or explaining in a casual voice *why* it
  was launched, is marketing and storytelling. Explaining how the feature
  is *supposed to work* is docs.

- **Deep internals of how the code works.** Focus on how the product
  changes for the end user instead.

  *Why:* Most users don't care how Coder's code is written; they care
  about inputs and outputs.

- **Duplicated third-party documentation** (Terraform, Amazon, Microsoft,
  Google, other vendors). Link to their docs instead.

  *Why:* Vendor docs are the source of truth; ours would immediately
  start drifting from them.

  *Exception:* Minimal Terraform teaching examples, as described in
  [What belongs in the docs](#what-belongs-in-the-docs).

## Routing table

When content doesn't belong in the docs, here's where it goes.

| Content type                             | Destination                            | Why                                                                                                                                                  |
|------------------------------------------|----------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------|
| Feature announcements                    | Blog                                   | Docs are version-scoped and factual; announcements are storytelling                                                                                  |
| Launch rationale ("why we built this")   | Blog                                   | Casual, narrative voice belongs on the blog                                                                                                          |
| Performance improvements                 | Changelog or blog                      | No change to how the user interacts with the product                                                                                                 |
| Release-by-release changes               | Changelog                              | The changelog is the record of what changed and when                                                                                                 |
| Known bugs or undesired behavior         | Changelog                              | Docs shouldn't highlight deficiencies (see exception above)                                                                                          |
| Troubleshooting ("when things go wrong") | Support KB (Pylon)                     | Support is primary owner of failure-mode content (Docs secondary where needed); docs own intended behavior and link to the KB via an embedded widget |
| Contributing guides                      | GitHub (or public Notion)              | Audience is contributors, not end users                                                                                                              |
| Style guides (code and docs)             | GitHub, with the codebase              | Same audience as contributing guides; enables CI enforcement                                                                                         |
| Third-party tool or cloud instructions   | Vendor docs (linked)                   | Vendor docs are the source of truth; ours would drift                                                                                                |
| Code internals or implementation detail  | Engineering docs (GitHub), if anywhere | End users care about inputs and outputs, not implementation                                                                                          |

## Open items

These items have been agreed in principle but the mechanics are still
under investigation. Update this section as they land.

- **In-docs troubleshooting migration.** Tracked in
  [DOCS-363](https://linear.app/codercom/issue/DOCS-363) (Urgent, cycle
  4). Audits the existing `## Troubleshooting` sections and dedicated
  troubleshooting pages under `docs/`, rewrites them for KB voice, and
  uploads them via the Pylon API. Until that work lands, link out to the
  relevant Pylon article from the page body; if no Pylon article exists
  yet, leave the existing inline troubleshooting in place rather than
  removing it.
- **Pylon KB widget implementation.** The direction is decided (embedded
  widget surfacing relevant KB articles per page or section); the
  mechanics are still under investigation.
- **Automated screenshot generation.** Today, doc-check only analyzes and
  comments; it creates nothing. The repo has Playwright e2e infrastructure
  under `site/e2e/`, so an agent workspace generating screenshots is
  plausible but unproven. If it becomes real, revisit loosening the
  screenshots policy. Until then, the screenshot rules in
  [What belongs in the docs](#what-belongs-in-the-docs) govern.
- **doc-check redirect suggestions.** When doc-check detects a moved or
  renamed page, it should suggest the exact `redirects.json` entry for
  `coder/coder.com` in a code block, so applying it is at most a
  copy-paste job.
