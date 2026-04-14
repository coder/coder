---
name: deep-review
description: "Multi-reviewer code review. Spawns domain-specific reviewers in parallel, cross-checks findings, posts a single structured GitHub review."
---

# Deep Review

Multi-reviewer code review. Spawns domain-specific reviewers in parallel, cross-checks their findings for contradictions and convergence, then posts a single structured GitHub review with inline comments.

## When to use this skill

- PRs touching 3+ subsystems, >500 lines, or requiring domain-specific expertise (security, concurrency, database).
- When you want independent perspectives cross-checked against each other, not just a single-pass review.

Use `.claude/skills/code-review/` for focused single-domain changes or quick single-pass reviews.

**Prerequisite:** This skill requires the ability to spawn parallel subagents. If your agent runtime cannot spawn subagents, use code-review instead.

**Severity scales:** Deep-review uses P0–P4 (consequence-based). Code-review uses 🔴🟡🔵. Both are valid; they serve different review depths. Approximate mapping: P0–P1 ≈ 🔴, P2 ≈ 🟡, P3–P4 ≈ 🔵.

## When NOT to use this skill

- Docs-only or config-only PRs (no code to structurally review). Use `.claude/skills/doc-check/` instead.
- Single-file changes under ~50 lines.
- The PR author asked for a quick review.

## 0. Proportionality check

Estimate scope before committing to a deep review. If the PR has fewer than 3 files and fewer than 100 lines changed, suggest code-review instead. If the PR is docs-only, suggest doc-check. Proceed only if the change warrants multi-reviewer analysis.

## 1. Scope the change

**Author independence.** Review with the same rigor regardless of who authored the PR. Don't soften findings because the author is the person who invoked this review, a maintainer, or a senior contributor. Don't harden findings because the author is a new contributor. The review's value comes from honest, consistent assessment.

Create the review output directory before anything else:

```sh
export REVIEW_DIR="/tmp/deep-review/$(date +%s)"
mkdir -p "$REVIEW_DIR"
```

**Re-review detection.** Fetch the PR context and save the diff for all reviewers:

```sh
go run ./.agents/skills/deep-review/scripts fetch-context \
  --pr {number} --repo {owner/repo} --output "$REVIEW_DIR/pr-context.json"

gh pr diff {number} > "$REVIEW_DIR/pr.diff"
```

The agent reads `$REVIEW_DIR/pr-context.json` to detect prior agent reviews. Scan the `reviews` entries for bodies containing P0–P4, `**Obs**`, or `**Nit**` patterns. If a prior agent review exists, the context file provides everything needed to classify carry-forward findings:

- `fetch-context` skips resolved threads, so findings from a prior round that were addressed simply don't appear in the output.
- Unresolved threads from prior agent reviews are the carry-forward findings.
- Author replies are threaded under them via `in_reply_to_id`.
- **Resolved/Acknowledged** = thread resolved on GitHub (excluded from output by `fetch-context`).
- **Contested** = unresolved thread with an author reply. The author disagreed or raised a constraint — engage with their argument before re-raising.
- **No response** = unresolved thread without an author reply.

Only **Contested** and **No response** findings carry forward to the new review. Do not re-raise resolved findings.

**Scope the diff.** Get the file list from the diff, PR, or user. Skim for intent and note which layers are touched (frontend, backend, database, auth, concurrency, tests, docs).

For each changed file, briefly check the surrounding context:

- Config files (package.json, tsconfig, vite.config, etc.): scan the existing entries for naming conventions and structural patterns.
- New files: check if an existing file could have been extended instead.
- Comments in the diff: do they explain why, or just restate what the code does?

## 2. Pick reviewers

Match reviewer roles to layers touched. The Test Auditor, Edge Case Analyst, and Contract Auditor always run. Conditional reviewers activate when their domain is touched.

### Tier 1 — Structural reviewers

| Role                 | Focus                                                       | When                                                        |
| -------------------- | ----------------------------------------------------------- | ----------------------------------------------------------- |
| Test Auditor         | Test authenticity, missing cases, readability               | Always                                                      |
| Edge Case Analyst    | Chaos testing, edge cases, hidden connections               | Always                                                      |
| Contract Auditor     | Contract fidelity, lifecycle completeness, semantic honesty | Always                                                      |
| Structural Analyst   | Implicit assumptions, class-of-bug elimination              | API design, type design, test structure, resource lifecycle |
| Performance Analyst  | Hot paths, resource exhaustion, allocation patterns         | Hot paths, loops, caches, resource lifecycle                |
| Database Reviewer    | PostgreSQL, data modeling, Go↔SQL boundary                  | Migrations, queries, schema, indexes                        |
| Security Reviewer    | Auth, attack surfaces, input handling                       | Auth, new endpoints, input handling, tokens, secrets        |
| Product Reviewer     | Over-engineering, feature justification                     | New features, new config surfaces                           |
| Frontend Reviewer    | UI state, render lifecycles, component design               | Frontend changes, UI components, API response shape changes |
| Duplication Checker  | Existing utilities, code reuse                              | New files, new helpers/utilities, new types or components   |
| Go Architect         | Package boundaries, API lifecycle, middleware               | Go code, API design, middleware, package boundaries         |
| Concurrency Reviewer | Goroutines, channels, locks, shutdown                       | Goroutines, channels, locks, context cancellation, shutdown |

### Tier 2 — Nit reviewers

| Role                   | Focus                                        | File filter                         |
| ---------------------- | -------------------------------------------- | ----------------------------------- |
| Modernization Reviewer | Language-level improvements, stdlib patterns | Per-language (see below)            |
| Style Reviewer         | Naming, comments, consistency                | `*.go` `*.ts` `*.tsx` `*.py` `*.sh` |

Tier 2 file filters:

- **Modernization Reviewer**: one instance per language present in the diff. Filter by extension:
  - Go: `*.go` — reference `.claude/docs/GO.md` before reviewing.
  - TypeScript: `*.ts` `*.tsx`: reference `.agents/skills/deep-review/references/typescript.md` before reviewing.
  - React: `*.tsx` `*.jsx`: reference `.agents/skills/deep-review/references/react.md` before reviewing.

  `.tsx` files match both TypeScript and React filters. Spawn both instances when the diff contains `.tsx` changes — TS covers language-level patterns; React covers component and hooks patterns. Before spawning, verify each instance's filter produces a non-empty diff. Skip instances whose filtered diff is empty.

- **Style Reviewer**: `*.go` `*.ts` `*.tsx` `*.py` `*.sh`

## 3. Spawn reviewers

Each reviewer writes findings to `$REVIEW_DIR/{role-name}.json` where `{role-name}` is the kebab-cased role name (e.g. `test-auditor`, `go-architect`). For Modernization Reviewer instances, qualify with the language: `modernization-reviewer-go.json`, `modernization-reviewer-ts.json`, `modernization-reviewer-react.json`. Reviewers use the `add-finding` script to produce structured JSON. The orchestrator does not read reviewer findings from the subagent return text — it reads the files in step 4.

Spawn all Tier 1 and Tier 2 reviewers in parallel. Give each reviewer a reference (PR number, branch name), not the diff content. The diff is already saved at `$REVIEW_DIR/pr.diff` — pass the path so reviewers don't each fetch their own copy. Reviewers are read-only — no worktrees needed.

**Tier 1 prompt:**

```text
You are the {Role Name} reviewer. Read your methodology in
`.agents/skills/deep-review/roles/{role-name}.md`.

Follow the review instructions in
`.agents/skills/deep-review/structural-reviewer-prompt.md`.

Review PR #{number}.
Diff file: {REVIEW_DIR}/pr.diff
Output file: {REVIEW_DIR}/{role-name}.json
```

**Tier 2 prompt:**

```text
You are the {Role Name} reviewer. Read your methodology in
`.agents/skills/deep-review/roles/{role-name}.md`.

Follow the review instructions in
`.agents/skills/deep-review/nit-reviewer-prompt.md`.

Review PR #{number}.
Diff file: {REVIEW_DIR}/pr.diff
File scope: {filter from step 2}.
Output file: {REVIEW_DIR}/{role-name}.json
```

For Modernization Reviewer instances, add the language reference after the methodology line:

- **Go:** `Read .claude/docs/GO.md as your Go language reference before reviewing.`
- **TypeScript:** `Read .agents/skills/deep-review/references/typescript.md as your TypeScript language reference before reviewing.`
- **React:** `Read .agents/skills/deep-review/references/react.md as your React language reference before reviewing.`

For re-reviews, append to both Tier 1 and Tier 2 prompts:

> Unresolved threads from prior agent reviews are in {REVIEW_DIR}/pr-context.json. Read the `reviews` and `review_threads` entries before reviewing. Do not re-raise findings whose threads are absent (they were resolved). For unresolved threads with author replies (contested findings), engage with the author's argument.

## 4. Cross-check findings

### 4a. Read findings from files

Compile all reviewer findings into a single structured inventory:

```sh
go run ./.agents/skills/deep-review/scripts compile-findings \
  --dir "$REVIEW_DIR" --output "$REVIEW_DIR/findings.json"
```

`findings.json` gives the orchestrator a structured inventory with convergence pre-identified (findings on the same file and line from different reviewers are grouped) and max severity pre-computed per group. This replaces manual one-file-at-a-time reading for the initial inventory.

Read individual reviewer `.json` files when you need the full evidence text during cross-check — `findings.json` contains summaries, not the complete reviewer prose. If a reviewer's file contains an empty JSON array, record that the reviewer had no findings and move on. If a file is missing (reviewer crashed or timed out), note the gap and proceed — do not stall or silently drop the reviewer's perspective.

After reading the compiled findings, proceed to cross-check.

### 4b. Cross-check

Handle Tier 1 and Tier 2 findings separately before merging.

**Tier 2 nit findings:** Apply a lighter filter. Drop nits that are purely subjective, that duplicate what a linter already enforces, or that the author clearly made intentionally. Keep nits that have a practical benefit (clearer name, better error message, obsolete stdlib usage). Surviving nits stay as Nit.

**Tier 1 structural findings:** Before producing the final review, look across all findings for:

- **Contradictions.** Two reviewers recommending opposite approaches. Flag both and note the conflict.
- **Interactions.** One finding that solves or worsens another (e.g. a refactor suggestion that addresses a separate cleanup concern). Link them.
- **Convergence.** Two or more reviewers flagging the same function or component from different angles. Don't just merge at max(severity) and don't treat convergence as headcount ("more reviewers = higher confidence in the same thing"). After listing the convergent findings, trace the consequence chain _across_ them. One reviewer flags a resource leak, another flags an unbounded hang, a third flags infinite retries on reconnect — the combination means a single failure leaves a permanent resource drain with no recovery. That combined consequence may deserve its own finding at higher severity than any individual one.
- **Async findings.** When a finding mentions setState after unmount, unused cancellation signals, or missing error handling near an await: (1) find the setState or callback, (2) trace what renders or fires as a result, (3) ask "if this fires after the user navigated away, what do they see?" If the answer is "nothing" (a ref update, a console.log), it's P3. If the answer is "a dialog opens" or "state corrupts," upgrade. The severity depends on what's at the END of the async chain, not the start.
- **Mechanism vs. consequence.** Reviewers describe findings using mechanism vocabulary ("unused parameter", "duplicated code", "test passes by coincidence"), not consequence vocabulary ("dialog opens in wrong view", "attacker can bypass check", "removing this code has no test to catch it"). The Contract Auditor and Structural Analyst tend to frame findings by consequence already — use their framing directly. For mechanism-framed findings from other reviewers, restate the consequence before accepting the severity. Consequences include UX bugs, security gaps, data corruption, and silent regressions — not just things users see on screen.
- **Weak evidence.** Findings that assert a problem without demonstrating it. Downgrade or drop.
- **Unnecessary novelty.** New files, new naming patterns, new abstractions where the existing codebase already has a convention. If no reviewer flagged it but you see it, add it. If a reviewer flagged it as an observation, evaluate whether it should be a finding.
- **Scope creep.** Suggestions that go beyond reviewing what changed into redesigning what exists. Downgrade to P4.
- **Structural alternatives.** One reviewer proposes a design that eliminates a documented tradeoff, while others have zero findings because the current approach "works." Don't discount this as an outlier or scope creep. A structural alternative that removes the need for a tradeoff can be the highest-value output of the review. Preserve it at its original severity — the author decides whether to adopt it, but they need enough signal to evaluate it.
- **Pre-existing behavior.** "Pre-existing" doesn't erase severity. Check whether the PR introduced new code (comments, branches, error messages) that describes or depends on the pre-existing behavior incorrectly. The new code is in scope even when the underlying behavior isn't.

For each finding **and observation**, apply the severity test in **both directions**. Observations are not exempt — a reviewer may underrate a convention violation or a missing guarantee as Obs when the consequence warrants P3+:

- Downgrade: "Is this actually less severe than stated?"
- Upgrade: "Could this be worse than stated?"

When the severity spread among reviewers exceeds one level, note it explicitly. Only credit reviewers at or above the posted severity. A finding that survived 2+ independent reviewers needs an explicit counter-argument to drop. "Low risk" is not a counter when the reviewers already addressed it in their evidence.

Before forwarding a nit, form an independent opinion on whether it improves the code. Before rejecting a nit, verify you can prove it wrong, not just argue it's debatable.

Drop findings that don't survive this check. Adjust severity where the cross-check changes the picture.

After filtering both tiers, check for overlap: a nit that points at the same line as a Tier 1 finding can be folded into that comment rather than posted separately.

### 4c. Quoting discipline

When a finding survives cross-check, the reviewer's technical evidence is the source of record. Do not paraphrase it.

**Convergent findings — sharpest first.** When multiple reviewers flag the same issue:

1. Rank the converging findings by evidence quality.
2. Start from the sharpest individual finding as the base text.
3. Layer in only what other reviewers contributed that the base didn't cover (a concrete detail, a preemptive counter, a stronger framing).
4. Attribute to the 2–3 reviewers with the strongest evidence, not all N who noticed the same thing.

**Single-reviewer findings.** Go back to the reviewer's file and copy the evidence verbatim. The orchestrator owns framing, severity assessment, and practical judgment — those are your words. The technical claim and code-level evidence are the reviewer's words.

A posted finding has two voices:

- **Reviewer voice** (quoted): the specific technical observation and code evidence exactly as the reviewer wrote it.
- **Orchestrator voice** (original): severity framing, practical judgment ("worth fixing now because..."), scenario building, and conversational tone.

If you need to adjust a finding's scope (e.g. the reviewer said "file.go:42" but the real issue is broader), say so explicitly rather than silently rewriting the evidence.

**Attribution must show severity spread.** When reviewers disagree on severity, the attribution should reflect that — not flatten everyone to the posted severity. Show each reviewer's individual severity: `*(Security Reviewer P1, Concurrency Reviewer P1, Test Auditor P2)*` not `*(Security Reviewer, Concurrency Reviewer, Test Auditor)*`.

**Integrity check.** Before posting, verify that quoted evidence in findings actually corresponds to content in the diff. This guards against garbled cross-references from the file-reading step.

## 5. Post the review

When reviewing a GitHub PR, post findings as a proper GitHub review with inline comments, not a single comment dump.

**Review body.** Open with a short, friendly summary: what the change does well, what the overall impression is, and how many findings follow. Call out good work when you see it. A review that only lists problems teaches authors to dread your comments.

```text
Clean approach to X. The Y handling is particularly well done.

A couple things to look at: 1 P2, 1 P3, 3 nits across 5 inline
comments.
```

For re-reviews (round 2+), open with what was addressed:

```text
Thanks for fixing the wire-format break and the naming issue.

Fresh review found one new issue: 1 P2 across 1 inline comment.
```

Keep the review body to 2–4 sentences. Don't use markdown headers in the body — they render oversized in GitHub's review UI.

**Inline comments.** Every finding is an inline comment, pinned to the most relevant file and line. For findings that span multiple files, pin to the primary file.

Inline comment format:

```text
**P{n}** One-sentence finding *(Reviewer Role)*

> Reviewer's evidence quoted verbatim from their file

Orchestrator's practical judgment: is this worth fixing now, or
is the current tradeoff acceptable? Scenario building, severity
reasoning, fix suggestions — these are your words.
```

For convergent findings (multiple reviewers, same issue):

```text
**P{n}** One-sentence finding *(Performance Analyst P1,
Contract Auditor P1, Test Auditor P2)*

> Sharpest reviewer's evidence as base text

> *Contract Auditor adds:* Additional detail from their file

Orchestrator's practical judgment.
```

For observations: `**Obs** One-sentence observation *(Role)* ...` For nits: `**Nit** One-sentence finding *(Role)* ...`

P3 findings and observations can be one-liners. Group multiple nits on the same file into one comment when they're co-located.

**Review event.** Always use `COMMENT`. Never use `REQUEST_CHANGES` — this isn't the norm in this repository. Never use `APPROVE` — approval is a human responsibility.

For P0 or P1 findings, add a note in the review body: "This review contains findings that may need attention before merge."

**Posting via the build-review scripts.** The orchestrator builds the review incrementally using the `build-review` and `post-review` scripts:

```sh
# Initialize the review.
go run ./.agents/skills/deep-review/scripts build-review init \
  --output "$REVIEW_DIR/review.json" \
  --body "Clean approach to X. 1 P2, 1 P3 across 2 inline comments."

# Add each finding as an inline comment.
go run ./.agents/skills/deep-review/scripts build-review comment \
  --output "$REVIEW_DIR/review.json" \
  --path "file.go" --line 42 \
  --body "**P1** Finding... *(Reviewer Role)*

> Evidence quoted verbatim

Orchestrator judgment."

# For re-reviews: reply to existing threads.
go run ./.agents/skills/deep-review/scripts build-review reply \
  --output "$REVIEW_DIR/review.json" \
  --in-reply-to 456 --body "Acknowledged."

# For re-reviews: resolve addressed threads.
go run ./.agents/skills/deep-review/scripts build-review resolve \
  --output "$REVIEW_DIR/review.json" \
  --thread-id "PRT_xyz789"

# Post.
go run ./.agents/skills/deep-review/scripts post-review \
  --pr {number} --repo {owner/repo} \
  --input "$REVIEW_DIR/review.json"
```

**Tone guidance.** Frame design concerns as questions: "Could we use X instead?" — be direct only for correctness issues. Hedge design, not bugs. Build concrete scenarios to make concerns tangible. When uncertain, say so. See `.claude/docs/PR_STYLE_GUIDE.md` for PR conventions.

## Follow-up

After posting the review, monitor the PR for author responses. If the author pushes fixes or responds to findings, consider running a re-review (this skill, starting from step 1 with the re-review detection path). Allow time for the author to address multiple findings before re-reviewing — don't trigger on each individual response.

During re-reviews, use `build-review resolve` to resolve threads for findings that were addressed. This keeps the PR conversation clean — resolved threads collapse on GitHub, making it easy for the author to see what's still outstanding.
