---
name: write-docs
description: Authoring workflow and guardrails for writing, moving, or restructuring Coder documentation under docs/. Points at the canonical content guidelines and prose style guide, then walks research, routing, Diátaxis mode, structure, pedagogy, and validation. Counterpart to the doc-check skill, which reviews changes for documentation needs.
---

# Documentation Authoring Skill

Author or edit user-facing documentation under `docs/` so it is correct,
correctly scoped, and approvable in as few review cycles as possible. This is
the counterpart to the `doc-check` skill: `doc-check` decides whether a change
needs docs; this skill covers writing them well.

> [!IMPORTANT]
> The **canonical** rules live outside this skill. Read them first; this skill
> only tells you how to apply them.
>
> - **Scope and routing** (does this belong in `docs/` at all, and where it
>   goes if not): [`docs/.style/content-guidelines.md`](../../../docs/.style/content-guidelines.md).
>   It governs on conflict.
> - **Prose and formatting:** the prose style guide at
>   [`docs/.style/style-guide/`](../../../docs/.style/style-guide/README.md).
>   Open it and apply it as a checklist. Do not write from memory; most style
>   churn in review comes from rules that already exist but were not applied.
> - **Agent-facing structure and research notes:**
>   [`.claude/docs/DOCS_STYLE_GUIDE.md`](../../docs/DOCS_STYLE_GUIDE.md).
>
> When this skill conflicts with the content guidelines, the content
> guidelines win.

## Goal

Most documentation review churn does not come from prose quality. It comes
from describing behavior that is wrong, content that does not belong in the
docs, or a page sequenced badly. This skill attacks those causes first, then
style.

## Workflow

1. **Establish ground truth before you write a sentence.** This is the
   highest-leverage step and the one most often skipped. It is the practical
   form of the content guidelines principle
   [Verify against the code; document exact values](../../../docs/.style/content-guidelines.md#verify-against-the-code-document-exact-values).
   - **Read the real source.** Open the actual template, config, code path,
     or CLI definition. Copy exact identifiers, defaults, file paths, option
     names, RBAC role names, thresholds, and API paths from the source, not
     from memory.
   - **Run the real thing.** Execute the commands in the same environment and
     image the reader will use. Capture real output and real error strings.
     Do not paraphrase an error you did not see. If you can only
     source-verify a value, say so and flag it for the reviewer rather than
     presenting a guess as fact. Programmatic content is a
     [testable CI surface](../../../docs/.style/content-guidelines.md#programmatic-content-is-a-testable-ci-surface).
   - **Learn the invariant.** For each behavioral claim, find the rule
     underneath it, so you can explain *why*, not just *what*, and not write
     something the maintainer knows is false.
   - **Read the issue, linked tickets, and referenced PRs.** Real constraints
     and intent often live there, not in the prose request.
   - **Confirm integrations that already work.** Do not invent setup steps
     for something the platform wires up for the user. When unsure whether a
     step is required, test both paths or ask, rather than padding the guide.
2. **Decide whether it belongs in the docs, and where.** Walk the
   [quick decision checklist](../../../docs/.style/content-guidelines.md#quick-decision-checklist).
   If it does not belong, route it (see [What not to write](#what-not-to-write)).
3. **Pick the Diátaxis mode and the manifest slot.** Choose one mode per page
   (tutorial, how-to guide, reference, or explanation) per
   the Diátaxis framework in the [content guidelines](../../../docs/.style/content-guidelines.md#follow-the-diátaxis-framework).
   One outcome per page. New pages MUST be added to `docs/manifest.json` under
   the right section, and the documentation lands in the same change as the
   feature.
4. **Draft with deliberate pedagogy** (see patterns below).
5. **Self-review and validate.** Apply the prose style guide with it open.
   Run `make lint/emdash`, markdownlint, and Vale. Run the commands and code
   in the page. Fix every inbound link you moved and add redirects for any
   rename (see [Structural rules to apply](#structural-rules-to-apply)).
6. **Open the PR.** Write the title and description per the
   [Pull Request Description Style Guide](../../docs/PR_STYLE_GUIDE.md), which
   also covers when to open as a draft and when to mark it ready for review.
   Keep the diff reviewable (see [Keep PRs reviewable](#keep-prs-reviewable)).

## Pedagogy patterns

- **Teach by discovery, but do not spoil the surprise.** A strong tutorial
  has the reader do the thing, observe the result (including a failure), and
  only then explains the mechanism and the fix. Front-loading the explanation
  removes the reason the reader believes it.
- **Frame code as an instruction, not decoration.** Every code block should
  answer "what do I do with this?" Prefer "Add this block to `main.tf`" over
  dropping a block the reader must infer they should paste. Do not show code
  for its own sake.
- **Keep full-file dumps out of the steps.** Inline only the diff the reader
  applies. If a complete reference file helps, put it in a collapsed block at
  the end, not in the middle of a step.
- **Respect the reader's tools.** Do not call a tool "the wrong fit" when it
  works with configuration. Describe what it costs and how to make it work.
- **Minimize cross-page travel for one task.** A tutorial that sends the
  reader to several other pages to finish a single task will be sent back.
  Inline the happy path; link out for depth, not for required steps.
- **Mirror parallel paths.** If the product has a UI and a CLI, show both for
  each step in a consistent structure so neither audience is stranded.
- **Show the truth in its lowest-maintenance form.** When two correct
  presentations exist, prefer the one that ages best. A hard-coded line number
  or a screenshot of text reads fine today and rots when the source changes. A
  screenshot still earns its place when a UI step genuinely needs to be seen;
  weigh the upkeep, and if you cannot capture one, flag the gap rather than
  omit it silently or treat "no screenshot" as a policy.
- **Orient the reader inside a series.** A multi-page series must say where the
  reader is and what comes next. End each page with a consistent next step (and
  Previous/Next where the engine supports it); never ship a page that
  dead-ends.

## What not to write

Do not put non-docs content in `docs/`. The canonical catalog of what to
exclude, where each item goes, and why is
[What does not belong in the docs](../../../docs/.style/content-guidelines.md#what-does-not-belong-in-the-docs)
plus the [routing table](../../../docs/.style/content-guidelines.md#routing-table).
Check it before adding a page. Do not reproduce the catalog here, so it cannot
drift from the source.

## Structural rules to apply

The canonical
[Structural rules](../../../docs/.style/content-guidelines.md#structural-rules)
cover the manifest entry, auto-generated content, Premium marking, renames
and redirects, and the emdash ban. Read them for the exact wording; the
pre-handoff checklist below turns them into pass/fail items. Two application
notes the canonical rules do not spell out:

- On a rename, pick the new link target by the specific page each sentence
  promises, not just the section hub, and confirm moved anchors still resolve.
- Keep the redirect PR in `coder/coder.com` in sync with the rename PR so the
  old public path never 404s between merges.

## Keep PRs reviewable

Large diffs get worse reviews. A reviewer who cannot hold the whole change in
their head will either rubber-stamp it or bounce it, and both cost more cycles
than splitting up front. Keep each docs PR focused, ideally under ~1,000 lines
changed whenever possible. For a multi-page series, prefer one page (or one
tightly scoped change) per PR, and stack or sequence them rather than shipping
the whole series as a single review.

## Anti-patterns observed in real review cycles

- Describing behavior you did not verify (guessed error strings, assumed auth
  flows, assumed persistence). The most expensive class of mistake.
- Code shown for its own sake, or a full file pasted into the middle of a
  step.
- A tutorial that makes the reader hop across pages to finish one task.
- A correct but heavy-handed example that belongs in a different doc.
- Spoiling a discovery-based lesson by explaining the mechanism first.
- Telling the reader their tool is wrong when it merely needs configuration.
- Brittle references that rot: hard-coded line numbers, or a screenshot
  standing in for text the reader could copy.
- Duplicating large content silently instead of flagging the maintenance
  cost to the reviewer.
- Treating the style guide as optional recall instead of a checklist you open
  and apply.

## Pre-handoff checklist

- [ ] Every factual claim is sourced from real code, a real run, or a linked
      ticket, not from assumption.
- [ ] Commands and code in the page were executed, or explicitly flagged as
      unverified for the reviewer.
- [ ] The content belongs in `docs/`; anything that does not was routed.
- [ ] One outcome per page, correct Diátaxis mode, added to
      `docs/manifest.json`.
- [ ] Prose style guide applied with it open; `make lint/emdash`,
      markdownlint, and Vale pass.
- [ ] Inbound links resolve; renames have redirects in `coder/coder.com`.
- [ ] Premium pages carry the title suffix and manifest state.
- [ ] Series pages orient the reader and link the next step; no dead-ends.
- [ ] The change is scoped for review: large or multi-page work is split into
      focused PRs (aim for under ~1,000 lines changed; one page per PR for a
      series).
- [ ] PR title and description follow the PR description style guide (including
      draft vs. ready-for-review).
- [ ] Maintenance tradeoffs (duplication, unverified claims) are disclosed to
      the reviewer, not hidden.

## Feeding lessons back

When a reviewer teaches a rule that is not yet in `docs/.style/`, add it there
in the same change set so the next author, human or model, starts from it
instead of rediscovering it in review. Shrinking review over time is the point
of this skill.
