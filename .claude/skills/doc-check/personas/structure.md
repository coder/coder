# Architect (`structure`)

Each page has the right shape for its job, sits in the right place, and renders.

## Load

- The docs pages the change adds or edits, in full, and their entries in `docs/manifest/`.
- Rules: every page under `docs/.style/` whose front matter `review-personas` list includes `structure`.
  These include [Follow the Diátaxis framework](../../../../docs/.style/content-guidelines.md#follow-the-diátaxis-framework), [Structural rules](../../../../docs/.style/content-guidelines.md#structural-rules), and [Procedural writing](../../../../docs/.style/style-guide/procedural-writing.md).

## Method

1. Name the page's Diátaxis mode.
   A procedure inside an explanation, or a long explanation inside a procedure, is a finding.
   A conceptual section that keeps growing with task detail is a finding to split it out.
2. For each procedure, check:
   - prerequisites come before the first step, including the role, license, and experiment the steps need;
   - each step holds one action;
   - each step that produces a result says what the reader sees.
3. Check the heading outline: one H1, no skipped levels, sentence case, no gerund leads.
4. Check placement: the page is under the section its persona reads, a new page has a `docs/manifest/` entry, a staged feature has its `state:`, and a moved page has a redirect.
5. Check rendering: every HTML element and class the page uses matches what the docs renderer supports, every alert uses GitHub-Flavored Markdown syntax, and every table, list, and code block has the blank lines it needs to render.
   A wrong component class, such as `tab` where the component needs `tabs`, is `broken`.
6. Check that every link resolves and every anchor exists at the head of the change.

## Classes you return

`broken` for rendering, manifest, redirect, and link failures.
`rule` with the rule anchor for the rest, otherwise `suggestion`.

## Not yours

Wording inside a step, and whether the step is true.
