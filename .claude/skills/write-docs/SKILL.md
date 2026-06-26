---
name: write-docs
description: Authoring workflow and guardrails for writing or editing Coder documentation under docs/. Points at the canonical content guidelines and prose style guide, then walks research, routing, Diataxis mode, structure, pedagogy, and validation. Counterpart to the doc-check skill, which reviews changes for documentation needs.
---

# Write Docs Skill

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
3. **Pick the Diataxis mode and the manifest slot.** Choose one mode per page
   (tutorial, how-to guide, reference, or explanation) per
   the Diataxis framework in the [content guidelines](../../../docs/.style/content-guidelines.md).
   One concept per page. New pages MUST be added to `docs/manifest.json` under
   the right section, and the documentation lands in the same change as the
   feature.
4. **Draft with deliberate pedagogy** (see patterns below).
5. **Self-review and validate.** Apply the prose style guide with it open.
   Run `make lint/emdash`, markdownlint, and Vale. Run the commands and code
   in the page. Fix every inbound link you moved and add redirects for any
   rename (see [Renames and moves](#renames-and-moves)).

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

## What not to write

Do not put these in `docs/`. Route them per the
[routing table](../../../docs/.style/content-guidelines.md#routing-table).

- **Predictive, timeless, or stale content.** Document only what applies to
  the current version. Do not predict the future or carry forward material
  that no longer applies.
- **Troubleshooting and failure-mode content.** Route to the support
  knowledge base (Pylon); Support owns it. Docs own intended behavior.
- **Feature announcements and launch rationale ("why we built this").**
  Route to the blog. Explaining how a feature is *supposed to work* is docs.
- **Duplicated third-party documentation** (Terraform, AWS, Microsoft,
  Google, other vendors). Link to their docs; theirs is the source of truth.
  The only exception is a minimal Terraform teaching example.
- **Contributing guides and style guides.** Keep with the code on GitHub.
- **Deep code internals.** Focus on inputs and outputs for the end user.
- **Known bugs whose intended behavior is not yet documented.** Route to the
  changelog (document expected behavior only when Coder agrees the behavior
  is wrong and the docs do not yet cover what should happen).

## Premium feature signaling

A page documenting a Premium feature needs **both**, or it is a defect:

1. The H1 title takes a `(Premium)` suffix, for example
   `# Template Insights (Premium)`.
2. The page's `docs/manifest.json` entry includes `"state": ["premium"]`.

## Renames and moves

1. Fix every inbound link to the old path. Choose the new target by intent,
   the specific page the sentence promises, not just the section hub. A moved
   section takes its anchor with it, so verify anchors still resolve.
2. Add a redirect in
   [`coder/coder.com:redirects.json`](https://github.com/coder/coder.com/blob/master/redirects.json)
   that maps the old public path to the new one, and open that PR alongside
   the rename. Do not rely on a directory index resolving the old path, and
   do not create a `docs/_redirects` file in this repo.

## No emdash, endash, or `--` as punctuation

This applies in prose, code blocks, comments, and string literals. Use
commas, semicolons, or periods, or restructure the sentence. For numeric
ranges use a plain hyphen (`0-100`). Enforced by `make lint/emdash`.

## Anti-patterns observed in real review cycles

- Describing behavior you did not verify (guessed error strings, assumed auth
  flows, assumed persistence). The most expensive class of mistake.
- Code shown for its own sake, or a full file pasted into the middle of a
  step.
- A tutorial that makes the reader hop across pages to finish one task.
- A correct but heavy-handed example that belongs in a different doc.
- Spoiling a discovery-based lesson by explaining the mechanism first.
- Telling the reader their tool is wrong when it merely needs configuration.
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
- [ ] One concept per page, correct Diataxis mode, added to
      `docs/manifest.json`.
- [ ] Prose style guide applied with it open; `make lint/emdash`,
      markdownlint, and Vale pass.
- [ ] Inbound links resolve; renames have redirects in `coder/coder.com`.
- [ ] Premium pages carry the title suffix and manifest state.
- [ ] Maintenance tradeoffs (duplication, unverified claims) are disclosed to
      the reviewer, not hidden.

## Feeding lessons back

When a reviewer teaches a rule that is not yet in `docs/.style/`, add it there
in the same change set so the next author, human or model, starts from it
instead of rediscovering it in review. Shrinking review over time is the point
of this skill.
