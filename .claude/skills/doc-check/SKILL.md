---
name: doc-check
description: Reviews a PR, branch, or local diff for documentation correctness and completeness. Checks that every docs claim is true in the code, that user-facing behavior has docs, and that new or edited docs follow the content guidelines, style guide, and contributing guides. Use before opening or updating a PR that touches docs/ or user-facing behavior, and in CI; to write or restructure docs, use write-docs.
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
> The rules live in `docs/.style/` and the contributing guides, not in this skill.
> [`docs/.style/content-guidelines.md`](../../../docs/.style/content-guidelines.md) governs what belongs in the docs, and the prose style guide is every page under `docs/.style/style-guide/`.
> [`.claude/docs/DOCS_STYLE_GUIDE.md`](../../docs/DOCS_STYLE_GUIDE.md) and [`contributing/documentation.md`](../../../contributing/documentation.md) apply where those are silent.
> [`contract.md`](./contract.md#which-rule-wins) sets the order when sources disagree.
> This skill and its persona files tell you how to apply those rules, and never restate them.

## How a run works

You are the orchestrator.
You build a review packet once, then sub-agents do the reviewing, each with only the inputs its job needs.
Small contexts are what make the shared model fast, so keep yours small too:

- Never read the persona files, the rule pages, or [`contract.md`](./contract.md). The sub-agents read them.
- Never read a docs page or a code file to review it. Read only what a step below tells you to.
- Never restate a persona's method in a spawn prompt.

| File                                 | Who reads it                                                     |
|--------------------------------------|------------------------------------------------------------------|
| This file                            | You                                                              |
| [`output.md`](./output.md)           | You, before you post anything and before you write the run trace |
| [`contract.md`](./contract.md)       | Every sub-agent, first                                           |
| [`personas/`](./personas/)           | The sub-agent that runs that persona                             |
| [`path-priors.md`](./path-priors.md) | The Cartographer                                                 |

## Modes

- **CI review.** The workflow prompt names the PR and the trigger. Post the review per [`output.md`](./output.md).
- **Dry run.** Run the CI review in full, but post nothing and run no command that writes to GitHub.
  The prompt can pin the head SHA, which you use instead of the PR's current head, and a skill ref, which you copy the skill from instead of the base branch.
  The rule sources still come from the base branch.
  End your final message with the review you would have posted, then the run trace.
- **Author self-check.** You are the agent that wrote the change. Review your branch diff against the base branch, fix every blocking finding, and report the rest to the user. Post nothing to GitHub.
- **Focused review.** The prompt asks for one lens, such as `focus:accuracy`. Queue only the change model, that persona, and the verifier. Say in the trace that the other agents did not run.

## Workflow

Do every step in order.
Skipping a step is allowed only where the step says so, and the trace names each skipped step and why.

### 1. Build the review packet

In CI and in a dry run, read this file from the base branch, such as with `git show origin/main:.claude/skills/doc-check/SKILL.md`, so a PR cannot change how it gets reviewed.
In a dry run with a skill ref, read it from that ref instead.

Set the variables, then run the commands from the repository root.
The packet path is unique per run, so overlapping runs never share files.

```sh
PR=<PR number>
BASE_REF=<baseRefName from gh pr view>
HEAD_SHA=<headRefOid from gh pr view, or the SHA the prompt pins>
git fetch origin "$BASE_REF" "pull/$PR/head"
git -c advice.detachedHead=false checkout --quiet "$HEAD_SHA"
BASE_SHA=$(git merge-base "origin/$BASE_REF" "$HEAD_SHA")
POLICY_SHA=$(git rev-parse "origin/$BASE_REF")
SKILL_SHA=$POLICY_SHA   # a dry run with a skill ref sets this to that ref's SHA
PACKET="${TMPDIR:-/tmp}/doc-check/$PR-$(printf '%.7s' "$HEAD_SHA")-$(date -u +%Y%m%dT%H%M%SZ)"
POLICY="$PACKET/policy"
mkdir -p "$POLICY" "$PACKET/out" "$PACKET/batches"
git archive "$SKILL_SHA" .claude/skills/doc-check | tar -x -C "$POLICY"
git archive "$POLICY_SHA" docs/.style .claude/docs/DOCS_STYLE_GUIDE.md contributing/documentation.md | tar -x -C "$POLICY"
gh pr view "$PR" --repo coder/coder --json number,title,body,author,baseRefName,headRefOid,files,comments,reviews > "$PACKET/pr.json"
gh api --paginate "repos/coder/coder/pulls/$PR/comments" \
  --jq '.[] | {id, path, line, user: .user.login, in_reply_to_id, body}' > "$PACKET/review-comments.jsonl"
git diff --no-color --no-ext-diff "$BASE_SHA" "$HEAD_SHA" > "$PACKET/diff.patch"
git diff --no-color --name-status "$BASE_SHA" "$HEAD_SHA" > "$PACKET/changed-files.txt"
git diff --no-color --no-ext-diff --unified=0 "$BASE_SHA" "$HEAD_SHA" -- 'docs/*.md' \
  ':(exclude)docs/reference/api' ':(exclude)docs/reference/cli' ':(exclude)docs/.style/style-guide' \
  | awk -f "$POLICY/.claude/skills/doc-check/changed-lines.awk" > "$PACKET/changed-lines.txt"
```

Redirect output into the packet as shown, and look at a file only through a count or a short `head`, so the data does not enter your context.
When the prompt pins a head SHA that the fetch did not bring in, fetch it with `git fetch origin <SHA>`.
In a dry run with a skill ref, which is a branch, run `git fetch origin <skill ref>` and set `SKILL_SHA=$(git rev-parse "origin/<skill ref>")` before the first `git archive`.

For a branch or local change without a PR, set `PR=local` and `HEAD_SHA=$(git rev-parse HEAD)`, fetch only the base branch, skip the `gh` commands, write `none` into `pr.json`, and leave `review-comments.jsonl` and `related-prs.txt` empty.
A commit that was never pushed has no CI results, so step 3 records the CI checks as not run.
In author self-check, set `POLICY` to the repository root instead of copying, and run the three `git diff` commands with `"$BASE_SHA"` alone in place of `"$BASE_SHA" "$HEAD_SHA"`, so uncommitted edits count.
Run `git add --intent-to-add` on new files first, because `git diff` skips untracked files.

With a PR, list open PRs that touch the same docs pages, for the Cartographer:

```sh
gh pr list --repo coder/coder --state open --search "<changed docs page path>" --json number,title --limit 10
```

Write the results to `related-prs.txt`, one PR per line, with the pages it shares with this change.

Then write `manifest.md` with these lines: `pr`, `mode`, `head_sha`, `base_ref`, `base_sha`, `policy_sha`, `skill_sha`, `repo` (the checkout path), `policy` (the `$POLICY` path), `packet` (the `$PACKET` path), and `max_parallel` (1 when you run the agents yourself).

### 2. Plan the queue

Classify the change from `changed-files.txt` and `changed-lines.txt` alone:

- **Docs changed:** any `docs/**/*.md` outside `docs/reference/api/` and `docs/reference/cli/`.
- **Prose in scope:** `changed-lines.txt` is not empty.
- **Code changed:** any file outside `docs/`.

Group the rule pages for the Copy Editor sweeps.
Each group stays under about 8,000 words, which keeps each sweep's context small:

```sh
cd "$POLICY" && {
  echo .claude/docs/DOCS_STYLE_GUIDE.md
  echo contributing/documentation.md
  for f in docs/.style/*.md docs/.style/style-guide/*.md; do
    awk 'NR == 1 && !/^---/ { exit } NR > 1 && /^---/ { exit } /^review-personas:/ { found = 1; exit } END { exit !found }' "$f" && echo "$f"
  done
} | while IFS= read -r f; do printf '%s %s\n' "$(wc -w < "$f")" "$f"; done \
  | awk -v max=8000 '{ if (sum + $1 > max && sum > 0) { n++; sum = 0 } sum += $1; print "sweep-" (n + 1), $2 }' \
  > "$PACKET/sweeps.txt"
```

Every page directly under `docs/.style/` or `docs/.style/style-guide/` that has a `review-personas` front matter field lands in a sweep, whatever personas it lists, so a new rule page there is covered when it has the field.
The two contributing guides have no field and are always swept.

Build the queue in this order, and write each skipped agent and its reason to the trace:

| Agent ID       | Persona                                                    | Skip when                                                        |
|----------------|------------------------------------------------------------|------------------------------------------------------------------|
| `change-model` | [Analyst](./personas/change-model.md)                      | No code changed. Write the change model yourself, per step 3     |
| `accuracy`     | [Fact Checker](./personas/accuracy.md)                     | Never                                                            |
| `coverage`     | [Cartographer](./personas/coverage.md)                     | Never                                                            |
| `consistency`  | [Continuity Editor](./personas/consistency.md)             | No docs changed and the change model lists no changed literal    |
| `structure`    | [Architect](./personas/structure.md)                       | No docs changed                                                  |
| `sweep-<n>`    | [Copy Editor](./personas/language.md), one per sweep group | No prose in scope                                                |
| `audience`     | [Reader Advocate](./personas/audience.md)                  | No docs changed                                                  |
| `learner`      | [Newcomer](./personas/learner.md)                          | No docs changed and the change model lists no user-facing change |
| `verify-<n>`   | [Skeptic](./personas/verifier.md), one per batch           | No finding reaches step 6                                        |

The persona ID of a sweep is `language`, and of a verifier batch is `verifier`.
Every other agent's persona ID is its agent ID.

A Copy Editor sweep can start at once.
Every other persona starts only after `change-model.md` exists.
Decide the skips that depend on the change model then, from its first two lines.

### 3. Run the deterministic checks

Write each result to `checks.md`, one line per check.
Install nothing: the review workspace blocks package registries and toolchain downloads, and CI already runs the linters.

- `make lint/emdash`, run here.
- The CI results for the head SHA, read with the API instead of run again:

  ```sh
  gh api --paginate "repos/coder/coder/commits/$HEAD_SHA/check-runs?per_page=100" \
    --jq '.check_runs[] | select(.name == "lint" or .name == "lint-docs") | [.id, .name, .status, (.conclusion // "")] | @tsv'
  gh api --paginate "repos/coder/coder/check-runs/<lint-docs ID>/annotations?per_page=100" \
    --jq '.[] | "\(.path):\(.start_line) \(.title): \(.message)"'
  ```

  `lint-docs` runs markdownlint, the table formatter, and Vale on changed Markdown, and `lint` runs `lint/docs-html`, `lint/docs-manifest`, and `lint/docs-redirects`.
  When a name has more than one run, use the newest one that is not `skipped`.
  Keep only annotations on changed files.
  A run that is missing or not finished is `pending`, never a pass. Read it again before step 6.
- In author self-check, run the linters on the changed Markdown files instead, with each output in the packet:

  ```sh
  pnpm exec markdownlint-cli2 <changed .md files> > "$PACKET/markdownlint.txt" 2>&1
  mise exec aqua:errata-ai/vale -- vale --no-exit <changed .md files> > "$PACKET/vale.txt" 2>&1
  make lint/docs-html lint/docs-manifest > "$PACKET/docs-lint.txt" 2>&1
  ```

  Vale exits 0 at every severity here, so read its output, not its exit code, and keep only the lines on changed lines.
  When Vale reports missing styles, run `make docs/.style/.vale-synced` once and run it again.

A check that fails on a line the change added or edited is a blocking finding.
Never contradict a check's output, and list every check that did not run in the trace.

When no code changed, write `change-model.md` yourself:

```text
user_facing_changes: 0
literals_changed: 0

Docs-only change. No product behavior changed. Check each docs claim against the code at the head.
```

### 4. Run the sub-agents

When you can spawn sub-agents and the prompt gives you a model configuration ID, run every queued agent as its own sub-agent with that ID.
Never let a sub-agent run on a different model than the prompt names, because that hides how the configured model performs.
Without a model configuration ID, run each agent yourself in queue order: read `contract.md` once, read each persona file only when you start that agent, and write each output file the same way a sub-agent would.

Spawn each agent with this prompt, filled in and nothing added.
The lines that every agent in the run shares come first, so the model server can reuse their cached prefix.

```text
You are a doc-check sub-agent. Follow the sub-agent contract before anything else:
<POLICY>/.claude/skills/doc-check/contract.md
Review packet: <PACKET>
Role: <persona ID>, defined in <POLICY>/.claude/skills/doc-check/personas/<persona ID>.md
Assignment: <agent ID>, <for a sweep its rule pages from sweeps.txt, for a verifier its batch file, otherwise none>
Output: <PACKET>/out/<agent ID>.yaml
```

Use the agent ID as the sub-agent title.
Add no summary of the change, no hints, and no method: the packet carries everything the agent needs, and anything you add pulls every agent toward your reading.

#### Concurrency

Keep at most `max_parallel` sub-agents running: 2, unless the prompt sets a number from 1 to 4.
The model server serves about 8 requests at once before it queues, and several reviews often run together, so more than this slows every review.
Verifier batches count against the same limit.

1. Start the first queued agents whose inputs are ready, up to the limit.
2. Wait on the running agent that started first, with `wait_agent` and a timeout of 600 seconds.
3. When it finishes, confirm its output file exists, start the next ready agent, and go back to step 2.
   A finished agent with no output file is `blocked`.
4. When a wait times out, wait on the next running agent instead, so one slow agent does not hold the queue.
5. After four timed-out waits on the same agent, send it `message_agent`, interrupting: `Stop now. Write your output file with what you finished, status budget_exhausted.`
   If it has not finished after one more wait, interrupt it, confirm it stopped, and record it as `incomplete`.
   Count it against the limit until it stops.

The platform does not enforce these limits, so follow them exactly.

### 5. Collect the results

Read each output file in `out/`.
For each Copy Editor sweep, compare its `rules` entries with the rule sections of each of its pages, listed with `awk -f "$POLICY/.claude/skills/doc-check/rule-sections.awk" <rule page>`.
A sweep with fewer entries than sections is `incomplete`, and the trace names the sections it missed.

### 6. Verify

First run these checks yourself, because they need a lookup, not judgment, and drop each finding that fails:

- **The quote exists.** The `quote` appears on the head at `file:line`, such as with `sed -n '<line>,+2p' <file>`. A quote that spans lines starts at `line`.
- **Not already reported in a thread.** No open doc-check review thread in `review-comments.jsonl` raises it.
- **Not already reported by a check.** No annotation in `checks.md` is on the same line and refers to the same rule page and anchor.

Then merge findings with the same `file`, `line`, and `rule`, keeping the one with more evidence.
Group what is left: findings about the same sentence form one group, and findings about the same missing page form one group.
A group holds every finding that could duplicate or contradict another, so groups never need to see each other.

Put the groups into verifier batches of at most 6 findings, never splitting a group, and write each batch to `batches/<n>.yaml`.
Run each batch as agent `verify-<n>` with persona [`verifier`](./personas/verifier.md), through the same queue and limit as step 4.
A batch that ends without finishing is retried once, split in two.
A finding that is still not verified after the retry is `unverified`: it is never posted as an inline comment, and the summary counts it.

Drop what the verifiers reject, and keep the count for the trace.
A finding the verifier rejects never reaches the author.

### 7. Decide and report

Apply [Blocking](#blocking) to each surviving finding, then follow [`output.md`](./output.md).
The outcome is `incomplete` when any agent ended `blocked`, `budget_exhausted`, or `incomplete`, when a sweep missed a section, or when any finding is `unverified`.
An incomplete review never says or implies the docs are fine.
Every run ends with the run trace, even when there are no findings.

## Blocking

A finding blocks the merge only when it is in one of these classes.
Everything else is a suggestion.

1. **Wrong.** A docs claim the code contradicts, including a configurable default presented as fixed, and an unsupported compliance or security claim.
2. **Missing.** Behavior the change adds or changes, that is user-facing per [Documentation lands with the change](../../../docs/.style/content-guidelines.md#documentation-lands-with-the-change), with no docs that explain it. This includes a broken user path, such as a step that needs a role the page never mentions.
3. **Broken.** A typo, markup that renders wrong or breaks a component (such as `<div class="tab">` where the component needs `tabs`), a failing deterministic check, a new page missing from `docs/manifest/`, a missing Premium state, a rename without a redirect.
4. **Rule-anchored.** A consistency, audience, structure, or language finding where all three hold:
   - the finding cites a rule by page and anchor from a source in [Which rule wins](./contract.md#which-rule-wins), or a contradicting line on another page;
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
- A conflict between two rule sources. Record it in the trace as an adjacent gap, so the guides get fixed.
- A design-friction note. Write one when the design makes a docs sentence hard to make accurate, such as one setting with several names, or a value inverted from the convention other tools use. Tie each note to the sentence. Write as many as the change earns, and re-evaluate them every round.

## What not to review

These changes have no documentation surface.
When every changed file falls in one of these classes, CI skips the review before this skill runs, so a diff in front of you has already passed that gate or a maintainer forced the review with the `doc-check` label.
Review only the user-facing part of a mixed diff.

- Auto-generated reference under `docs/reference/cli/` and `docs/reference/api/`. Review the source comment or CLI definition that generates it instead.
- Internal-only refactors, test-only changes, CI and tooling changes that change no user-facing surface, dependency bumps without behavior changes, and pure code moves.
- Features guarded by an unsafe experiment flag. Refer to [Experiments versus feature stages](../../../docs/.style/content-guidelines.md#experiments-versus-feature-stages). A safe experiment or an Early Access feature needs at least a single page.
