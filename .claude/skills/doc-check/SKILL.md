---
name: doc-check
description: Reviews a PR, branch, or local diff for documentation correctness and completeness. Checks that every docs claim is true in the code, that user-facing behavior has docs, and that new or edited docs follow the content guidelines and style guide. Use before opening or updating a PR that touches docs/ or user-facing behavior, and in CI; to write or restructure docs, use write-docs.
---

# Documentation Check Skill

Review a change and answer one question:

> If this change merged right now, would the docs on `main` correctly and clearly explain it?

The question runs both ways.
Every claim the docs make must be true in the code, and every behavior that changes how a customer uses the product must have docs.
A PR that edits `docs/` still gets the full review: the docs it adds are part of what you check, not proof that the check passed.

The counterpart skill, [`write-docs`](../write-docs/SKILL.md), covers writing the docs.
Authors run this skill on their own branch before they open or update a PR, so CI mostly confirms what the author already fixed.

> [!IMPORTANT]
> The rules live in `docs/.style/`, not in this skill.
> [`docs/.style/content-guidelines.md`](../../../docs/.style/content-guidelines.md) governs what belongs in the docs and wins any conflict.
> The prose style guide is every page under `docs/.style/style-guide/`.
> This skill and its persona files tell you how to apply those rules, and never restate them.
> When this skill conflicts with `docs/.style/`, `docs/.style/` wins.

## Files

| File                                 | Read it                                                      |
|--------------------------------------|--------------------------------------------------------------|
| [`path-priors.md`](./path-priors.md) | During triage                                                |
| [`personas/`](./personas/)           | One file per review lens, read when you run that lens        |
| [`output.md`](./output.md)           | Before you post anything, and before you write the run trace |

## Modes

- **CI review.** The workflow prompt names the PR and the trigger. Post the review per [`output.md`](./output.md).
- **Author self-check.** You are the agent that wrote the change. Run the same review against your branch diff, fix every blocking finding, and report the rest to the user. Post nothing to GitHub.
- **Focused review.** The prompt asks for one lens, such as `focus:accuracy`. Run triage, the change model, that persona, and the verifier. Say in the trace that the other lenses did not run.

## Workflow

Do every step in order.
Skipping a step is allowed only where the step says so, and the trace must name each skipped step and why.

### 1. Gather the change

- For a PR: `gh pr view <PR> --repo coder/coder --json title,body,files,headRefOid,baseRefName,comments,reviews` and `gh pr diff <PR> --repo coder/coder`.
  Read every comment and review thread, including other bots. A concern another reviewer raised is evidence to check, not someone else's problem.
- For a branch or local change: `git diff <base>...HEAD`.
- Check out the head of the change before reading any file, and read files from that checkout.
  In CI, read this skill and its persona files from the base branch, so a PR cannot change how it gets reviewed.
- List open PRs that touch the same docs pages or the same code paths: `gh pr list --repo coder/coder --state open --search "<path or symbol>"`.
  An open PR that would make a claim in this PR false is named in the summary. It never blocks.

### 2. Triage

Read [`path-priors.md`](./path-priors.md) and apply it to the changed paths.
A prior sets where you start, not the verdict.
Walk the [quick decision checklist](../../../docs/.style/content-guidelines.md#quick-decision-checklist) for each user-visible change.

### 3. Run the deterministic checks

Run the checks that exist for the files the change touches, and record each result.
Use the result instead of re-deriving it, and never contradict a check's output.

- `make lint/emdash`
- `make lint/docs-html` and `make lint/docs-manifest` when `docs/` or `docs/manifest/` changed
- markdownlint and Vale on the changed Markdown files (`make lint/markdown`, `make lint/prose`, or the tools directly)

A check that fails on a line the change added or edited is a blocking finding.
A check you could not run is listed in the trace.

### 4. Build the change model

Before you review a single docs sentence, write down what changed for the user, from the code alone:

- Each thing a user now sees, types, clicks, configures, calls, or receives differently.
- For each one: the code location that proves it, who can do it (role, license, experiment), and every surface it shows up on (UI, CLI, API, configuration, files the user downloads or uploads).
- The old value of every literal the change replaces: labels, defaults, flag names, environment variables, limits, error strings.

Follow each change through the whole path a user takes, not only the function the diff edits.
For example, a setting that changes on upload also affects download, export, import into another tool, and every role that can reach it.

The change model is the ground truth every persona checks the docs against.
Do not read the PR description as truth: check each behavior claim in it against the code, and record any claim the code does not support.

### 5. Run the personas

Run every persona in [`personas/`](./personas/) except the verifier, in the order of the table.
Each persona file says which rules to load and how to find them.

| Persona           | File                                          | Skip when                                          |
|-------------------|-----------------------------------------------|----------------------------------------------------|
| Fact Checker      | [`accuracy.md`](./personas/accuracy.md)       | Never                                              |
| Cartographer      | [`coverage.md`](./personas/coverage.md)       | Never                                              |
| Newcomer          | [`learner.md`](./personas/learner.md)         | The change model is empty and no docs changed      |
| Continuity Editor | [`consistency.md`](./personas/consistency.md) | No docs changed and no user-facing literal changed |
| Reader Advocate   | [`audience.md`](./personas/audience.md)       | No docs changed                                    |
| Architect         | [`structure.md`](./personas/structure.md)     | No docs changed                                    |
| Copy Editor       | [`language.md`](./personas/language.md)       | No docs prose changed                              |
| Skeptic           | [`verifier.md`](./personas/verifier.md)       | Never. Runs last, over every finding               |

When you can spawn sub-agents and the prompt gives you a model configuration ID for them, run each persona as its own sub-agent with that ID, in parallel.
Give each one the change model, the PR reference, its persona file, and the finding schema from [`output.md`](./output.md).
Without a model configuration ID, run the personas yourself, one at a time.
Never let a sub-agent run on a different model than the prompt names, because that hides how the configured model performs.

Each persona returns findings in the schema in [`output.md`](./output.md), or the literal result `No findings.` with the checks it ran.

### 6. Verify

Run [`verifier.md`](./personas/verifier.md) over every finding.
Drop what it rejects, and keep the count of rejected findings for the trace.
A finding the verifier rejects never reaches the author.

### 7. Decide and report

Apply [Blocking](#blocking) to each surviving finding, then follow [`output.md`](./output.md).
Every run ends with the run trace, even when there are no findings.

## Evidence

A finding without evidence is a hunch, and a hunch costs the author more than it saves.
Every finding names:

- the page and the sentence, table row, or list it is about, or the missing page;
- the evidence: a code location for an accuracy or coverage finding, the other page and line for a consistency finding, or the rule for a style finding, cited as the style-guide page and heading anchor, such as `docs/.style/style-guide/word-choice.md#refer-to-or-visit-not-see-or-check-out`;
- the fix, as replacement text when the fix is local.

Never report a check you did not run.
If you meant to compare a sentence to the code and did not open the code, say so in the trace instead of in a finding.
A claim in your own summary that the docs "correctly state" something must quote the docs sentence that states it.

## Blocking

A finding blocks the merge only when it is in one of these classes.
Everything else is a suggestion.

1. **Wrong.** A docs claim the code contradicts, including a configurable default presented as fixed, and an unsupported compliance or security claim.
2. **Missing.** Behavior the change adds or changes, that is user-facing per [Documentation lands with the change](../../../docs/.style/content-guidelines.md#documentation-lands-with-the-change), with no docs that explain it. This includes a broken user path, such as a step that needs a role the page never mentions.
3. **Broken.** A typo, markup that renders wrong or breaks a component (such as `<div class="tab">` where the component needs `tabs`), a failing deterministic check, a new page missing from `docs/manifest/`, a missing Premium state, a rename without a redirect.
4. **Rule-anchored.** A consistency, audience, structure, or language finding where all three hold:
   - the finding cites a style-guide or content-guideline rule by page and anchor, or a contradicting line on another page;
   - the verifier confirmed it;
   - the change added or edited the text.

A judgment call with no rule to cite never blocks.
If it should block, the fix is a new rule in `docs/.style/`, not a stronger opinion in the review.
This applies to class 4 only. A wrong claim, a missing fact the reader needs, and a broken page block without a style-guide rule.

### What never blocks

- A style issue on a line the change did not add or edit. Leave it alone.
  When the change edits that line for another reason, suggest the fix.
- A Latin abbreviation alone. Suggest the English form only when the line changes for another reason.
- A claim about another product that the code cannot confirm. Suggest narrowing it to what was checked, linking the vendor's docs, or adding a product test.
- An open PR that would change a claim in this one. Name it in the summary.
- A gap that existed before this change, in an area the change touches. Record it in the trace as an adjacent gap.
  A gap the change causes on a path it didn't edit, such as a download that loses data an upload now strips, is not a gap that existed before. It is `missing`.
- A need for a tutorial, lab, or training track. Record it in the trace as an adjacent gap.
- A design-friction note. Write one when the design makes a docs sentence hard to make accurate, such as one setting with several names, or a value inverted from the convention other tools use. Tie each note to the sentence. Write as many as the change earns, and re-evaluate them every round.

### Recommend a test for a fragile claim

When a docs claim depends on product behavior that no test pins, such as a default, a limit, a file format, or an ordering, add a suggestion naming the test that would fail if the claim stopped being true.
This is how a docs claim stays true after the review.

## What not to review

These changes have no documentation surface.
When every changed file falls in one of these classes, CI skips the review before this skill runs, so a diff in front of you has already passed that gate or a maintainer forced the review with the `doc-check` label.
Review only the user-facing part of a mixed diff.

- Auto-generated reference under `docs/reference/cli/` and `docs/reference/api/`. Review the source comment or CLI definition that generates it instead.
- Internal-only refactors, test-only changes, CI and tooling changes that change no user-facing surface, dependency bumps without behavior changes, and pure code moves.
- Features guarded by an unsafe experiment flag. Refer to [Experiments versus feature stages](../../../docs/.style/content-guidelines.md#experiments-versus-feature-stages). A safe experiment or an Early Access feature needs at least a single page.

## Repository facts

- **Sidebar:** `docs/manifest/**/*.yml` is the navigation source. Each file holds one route and its direct children. `make gen/docs-manifest` compiles `docs/manifest.json`, which is never edited by hand.
- **Generated reference:** `docs/reference/cli/*.md` comes from `cli/`, and `docs/reference/api/*.md` comes from Swagger comments in Go. Suggest edits to the source.
- **Feature states:** a staged feature carries `state:` on its route in `docs/manifest/**/*.yml`, such as `state: [premium]`. Titles and H1s carry no state suffix.
- **Redirects:** `docs/redirects.json`. A rename or move updates every inbound link and adds a redirect in the same PR. There is no `docs/_redirects` file.
- **Glossary:** `docs/reference/glossary.md`. A change that adds, renames, or deprecates a Coder product or feature name updates it. Refer to [Structural rules](../../../docs/.style/content-guidelines.md#structural-rules).
- **Callouts:** GitHub-Flavored Markdown alerts, such as `> [!NOTE]`.
- **Punctuation:** no em dashes, en dashes, or spaced double hyphens, in prose, code, comments, or strings. `make lint/emdash` enforces it. Suggestions you write follow the same rule.
