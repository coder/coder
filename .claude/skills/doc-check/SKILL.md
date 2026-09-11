---
name: doc-check
description: Checks if code changes require documentation updates
---

# Documentation Check Skill

Review code changes and determine if documentation updates or new
documentation is needed. This skill decides *whether* a change needs docs;
its counterpart, the [`write-docs` skill](../write-docs/SKILL.md), covers
writing them.

> [!IMPORTANT]
> The **canonical** rules for what belongs in the Coder docs (and what
> doesn't) live in
> [`docs/.style/content-guidelines.md`](../../../docs/.style/content-guidelines.md).
> Read that first. When this skill conflicts with the content
> guidelines, the content guidelines govern.

## Workflow

1. **Get the code changes.** Use the method provided in the prompt, or if
   none specified:
   - For a PR: `gh pr diff <PR_NUMBER> --repo coder/coder`
   - For local changes: `git diff main` or `git diff --staged`
   - For a branch: `git diff main...<branch>`

2. **Triage the diff.** Walk the
   [quick decision checklist](../../../docs/.style/content-guidelines.md#quick-decision-checklist)
   in the content guidelines. Most non-user-facing diffs route out of
   the docs entirely; see [What not to comment on](#what-not-to-comment-on).

3. **Understand the scope.** Consider what changed:
   - Is this user-facing or internal?
   - Does it change behavior, APIs, CLI flags, or configuration?
   - Even for "internal" or "chore" changes, always verify the actual
     diff.

4. **Search the docs.** Find related content in `docs/`.

5. **Decide what's needed.** Consider:
   - Do existing docs need updates to match the code?
   - Is new documentation needed for undocumented features?
   - Or is everything already covered?

6. **Report findings.** Use the method provided in the prompt, or if none
   specified, summarize findings directly. When the prompt asks for a
   comment on a pull request, follow [Writing the comment](#writing-the-comment).

## Evidence discipline

Follow this order on every review.

1. **List what the user experiences, not what files changed.**
   "User-facing" means what a user sees, types, clicks, or receives.
   Dashboard code and wiring code count when they change what the user
   experiences. A new label, a new tab, a new required field, a new
   setting, and a removed control all count.
2. **Check two kinds of page per item.** For each item on that list, check
   the conceptual guide for that area, and any page that enumerates the
   things this change adds to or removes from (a table of settings, a list
   of tabs, a list of fields). An auto-generated CLI or API reference
   never closes a gap in a conceptual guide. Treat the reference and the
   guide as two separate checks.
3. **Search for the old fact, do not read only the diff.** List every
   literal the diff changes: default values, flag names, env var names,
   thresholds, UI labels. Search the whole docs tree for each **old**
   literal before you decide:

   ```sh
   grep -rn '<the old literal>' docs/ | grep -v '^docs/reference/'
   ```

   Search the value as a number and as prose, because a guide can spell it
   out ("thirty days" as well as `30d`). A hit outside `docs/reference/` is
   a gap: this PR regenerating a reference page never fixes a conceptual
   guide. Report each search you ran and what it returned. A review that
   inspects only the files in the diff cannot find this class of gap, which
   is the most common real one, so it is not a review.

   If no page states the fact and the content guidelines require it, that
   is also a gap. If no page states it and the guidelines do not require
   it, that is not a gap: absence alone is not drift.
4. **A PR that documents itself needs nothing more.** Judge the state
   after the whole diff lands. If the diff already adds or fixes the
   documentation that its own code change requires, the requirement is
   satisfied inside the PR. Post no comment.
5. **Write the evidence before the verdict.** For each user-facing change,
   state the change, the searches you ran, the page you checked, and what
   you found on it. Then state whether that evidence shows a real
   unresolved gap, and comment only when it does. A verdict that
   contradicts your own evidence is the most common failure mode on this
   job, so read both once more before you post.

   Staying silent is a finding too, and it earns the same evidence. Post
   nothing only after every search in step 3 came back empty outside
   `docs/reference/`. "The diff already updates its own reference page" is
   not a reason to skip the search.

## Writing the comment

### A finding needs a page and a sentence

Name the page, and name the sentence that is now wrong or the list that is
now missing an entry. If you cannot name both, you have a hunch, not a
finding, and a hunch costs the author more than it saves.

Two habits produce weak findings:

- **Documenting the interface.** A button, a filter preset, or a dialog is
  not a documented surface on its own. Flag it only when a page already
  enumerates the thing it belongs to, such as a table of settings or a
  list of filters.
- **Filing on the nearest page instead of the right one.** An
  admin-facing change does not belong on an agents page because that page
  happens to mention a similar option. When no page is the right home,
  say so in one sentence and file nothing.

One surface earns one item. Do not split a single change into a required
item plus two nearby suggestions.

### Checkboxes are work, not opinions

Every `[ ]` is work the author owes. Anything optional belongs in the
sentence under an item, or nowhere. An item that says "consider" or "not
strictly required" is not an item.

### Every item carries a link

An item names a page, so it can always link that page. Give the published
URL, which is `https://coder.com/docs/` plus the path with the `docs/`
prefix and the `.md` suffix removed. `docs/ai-coder/ai-gateway/reference.md`
becomes `https://coder.com/docs/ai-coder/ai-gateway/reference`.

Write the path in backticks so it is greppable, then link it, so a reader
can open the page in one click:

```markdown
- [ ] `docs/ai-coder/ai-gateway/reference.md` ([open](https://coder.com/docs/ai-coder/ai-gateway/reference)) - What needs to change
```

A page this pull request creates has no published URL yet. Name the path in
backticks alone and say the page is new.

### Links resolve on GitHub, not in the docs tree

A relative docs link resolves against the repository in a comment and
404s. Write the path in backticks, or link the published page in full,
such as `https://coder.com/docs/reference/api/enterprise`.

Link an anchor only when that heading exists on the base branch today. A
heading this pull request generates does not exist yet, so name the
endpoint or section in words instead.

### The marker is not optional

The comment ends with `<!-- doc-check-sticky -->`, on its own line, every
time. It is how the next review finds this comment instead of posting a
second one, and how the Slack notice knows a review had findings. A
comment without it reads as silence to everything downstream.

After you post or edit, read the comment back and confirm the marker is
there. If it is not, edit the comment to add it. This happened on
`coder/coder#28723`: the review found a real gap, posted it without the
marker, and the notice said "No docs needed".

### One comment per pull request

Search the pull request for `<!-- doc-check-sticky -->` and edit that
comment instead of adding another. Search again immediately before you
post: a comment you wrote earlier in this same review counts, and reviews
of one pull request can overlap. Edit it, never post a second.

When a comment already exists, compare your findings against it. Check off
`[x]` items that are now addressed, strike through items the code reverted,
and add `[ ]` items for new gaps. If an item is checked but you cannot
verify the documentation landed, add a warning note below it. If nothing
meaningful changed, leave the comment alone.

### Comment format

Include only the sections that apply.

```markdown
## Documentation Check

### Updates Needed
- [ ] `docs/path/file.md` ([open](https://coder.com/docs/path/file)) - What needs to change
- [x] `docs/other/file.md` ([open](https://coder.com/docs/other/file)) - This was addressed
- ~~`docs/removed.md` - No longer needed~~ *(reverted in abc123)*

### New Documentation Needed
- [ ] `docs/suggested/path.md` - What should be documented, on a page that does not exist yet
  > ⚠️ *Checked but no corresponding documentation changes found in this PR*

---
*Automated review via [Coder Agents](https://coder.com/docs/ai-coder/agents)*
<!-- doc-check-sticky -->
```

Keep to this structure. Do not add sections it does not have, such as an
evidence block. The evidence belongs in your answer, not in the author's
comment.

The `<!-- doc-check-sticky -->` marker goes last, so the next review can
find this comment.

## What to Check

- **Accuracy**: Does documentation match current code behavior?
- **Completeness**: Are new features or options documented?
- **Examples**: Do code examples still work?
- **CLI/API changes**: Are new flags, endpoints, or options documented?
- **Configuration**: Are new environment variables or settings documented?
- **Breaking changes**: Are migration steps documented if needed?
- **Evidence versus claim**: See
  [Evidence versus claim](#evidence-versus-claim) below.
- **Premium features**: See [Premium feature signaling](#premium-feature-signaling)
  below.
- **Renames or moves**: See [Renames and moves require redirects](#renames-and-moves-require-redirects)
  below.
- **Terminology and the glossary**: Does the change introduce, rename, or
  deprecate a Coder product or feature name? If so,
  `docs/reference/glossary.md` needs a matching entry. See
  [Glossary and terminology](#glossary-and-terminology) below.

## What not to comment on

Do not produce sticky-comment suggestions for these classes of change.
They have no user-visible documentation surface.

- **Auto-generated CLI docs** under `docs/reference/cli/`. These are
  generated from Go code under `cli/`; suggest edits to the CLI
  definitions instead.
- **Internal-only refactors** with no user-visible behavior change.
- **Test-only changes** (new tests, refactored tests, fixtures).
- **CI, release, or tooling commits** that don't change user-facing
  surfaces. This includes workflow YAML, Makefile internals, formatter
  configs, and lint configs.
- **Dependency bumps** without behavior changes.
- **Pure code reorganizations** (moves, renames, package restructuring
  with no API or behavior change).
- **Features guarded by an unsafe experiment flag.** Features behind an
  unsafe experiment are not designed for users yet and may be reverted.
  See
  [Experiments versus feature stages](../../../docs/.style/content-guidelines.md#experiments-versus-feature-stages)
  in the content guidelines for the experiment-vs-stage distinction. A
  safe experiment or an Early Access feature does need at least a
  single-page doc, so don't apply this rule to those.

If a diff is a mix of one of the above with a user-facing change, comment
only on the user-facing portion.

## Key Documentation Info

- **`docs/manifest.json`** is the navigation structure; new pages MUST be
  added here.
- **`docs/reference/cli/*.md`** is auto-generated from Go code. Don't
  edit directly.
- **`docs/.style/content-guidelines.md`** is the canonical source for
  what belongs in the docs.

### Premium feature signaling

A page documenting a Premium feature requires **both** of the following.
Missing either one is a defect:

1. The H1 title takes a `(Premium)` suffix. Example:
   `# Template Insights (Premium)`.
2. The page's `docs/manifest.json` entry includes `"state": ["premium"]`.

### Evidence versus claim

This is the canonical rule
[Evidence justifies a claim; it does not belong in the claim](../../../docs/.style/content-guidelines.md#evidence-justifies-a-claim-it-does-not-belong-in-the-claim)
in the content guidelines; the content guidelines govern, so read the rule
there. On a page **this change adds or edits**, flag an implementation
identifier the reader neither types nor receives in that page's task, whether
the diff adds the identifier or leaves it on a line this change touches; the
canonical rule's ladder decides the replacement. Work from the page content
in the diff. doc-check does not see commit messages or PR comments, so it
does not police whether a stripped identifier was disclosed; that pointer is
the author's to provide for the human reviewer. A pre-existing violation on a
page the diff does not touch is not this change's finding; mention it as
informational context at most, without demanding a fix from this author.

### No emdash, endash, or ` -- ` as punctuation

This applies in docs prose, code blocks, comments, and string literals.
Use commas, semicolons, or periods, or restructure the sentence. For
numeric ranges, use a plain hyphen (e.g., `0-100`). The rule is enforced
by `make lint/emdash`, but the doc-check skill should also flag
violations it generates or suggests.

### Renames and moves require redirects

Redirects for [coder.com/docs](https://coder.com/docs) are configured in
a separate repo, not in this one. When a doc page is renamed or moved:

1. Update every link that relies on the old location.
2. Add an entry to
   [`coder/coder.com:redirects.json`](https://github.com/coder/coder.com/blob/master/redirects.json)
   that maps the old path to the new one. Open that PR alongside the
   `coder/coder` rename PR.

Do not create a `docs/_redirects` file in this repo; that format isn't
processed by coder.com.

### Glossary and terminology

The [glossary](../../../docs/reference/glossary.md) defines Coder-specific
product and feature names, including collisions like the several senses of
"agent". It drifts when the product's vocabulary changes and the page
doesn't. Flag a glossary update when a change:

- Adds a Coder product or feature name that isn't in the glossary yet.
- Renames one. The entry should keep the former name (for example,
  "previously named ...").
- Deprecates one. The entry should say so and name the replacement.

This is the canonical rule in
[Structural rules](../../../docs/.style/content-guidelines.md#structural-rules);
the content guidelines govern. Don't flag generic lowercase concepts or
internal-only identifiers with no user-facing surface; they don't earn a
glossary entry.

## Coder-specific patterns

### Callouts

Use GitHub-Flavored Markdown alerts:

```markdown
> [!NOTE]
> Additional helpful information.

> [!WARNING]
> Important warning about potential issues.

> [!TIP]
> Helpful tip for users.
```

### CLI Documentation

CLI docs in `docs/reference/cli/` are auto-generated. Don't suggest
editing them directly. Changes should be made in the Go code that
defines the CLI commands (typically the `cli/` directory).

### Code Examples

Use `sh` for shell commands:

```sh
coder server --flag-name value
```
