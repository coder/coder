# Architect (`structure`)

Each page has the right shape for its job, sits in the right place, and renders.

Budget: 25 tool calls.

## Load

- The docs pages the change adds or edits, in full, at the head, and their entries in `docs/manifest/`.
- `changed-files.txt`, `checks.md`, and the repository facts in the contract.
- Rules: only the sections headed "Follow the Diátaxis framework" and "Structural rules", per [Finding your rule sections](../contract.md#finding-your-rule-sections).

The Copy Editor sweeps apply the formatting, heading, and procedural-writing rules line by line.
Your job is the page as a whole, and the mechanics that need other files.

## Method

1. Name each page's Diátaxis mode.
   A procedure inside an explanation, or a long explanation inside a procedure, is a finding.
   A conceptual section that keeps growing with task detail is a finding to split it out.
2. For each procedure, check that its prerequisites come before the first step, including the role, license, and experiment the steps need.
3. Check the heading outline with `grep -n '^#' <page>`: one H1 and no skipped levels.
   Skip what `checks.md` already reports.
4. Check placement: the page is under the section its reader reads, a new page has a `docs/manifest/` entry, a staged feature has its `state:`, and a moved or deleted page has a redirect in `docs/redirects.json`.
5. Check rendering: every HTML element and class the page uses matches what other docs pages use for the same component, every alert uses GitHub-Flavored Markdown syntax, and every table, list, and code block has the blank lines it needs to render.
   A wrong component class, such as `tab` where the component needs `tabs`, is `broken`.
6. For each link the change adds or edits, check that the target file exists at the head and that its anchor exists, per the heading-anchor fact in the contract.
   A missing target or anchor is `broken`.

## Classes you return

`broken` for rendering, manifest, redirect, and link failures.
`rule` with the rule anchor for the rest, otherwise `suggestion`.

## Not yours

Wording inside a step, and whether the step is true.
