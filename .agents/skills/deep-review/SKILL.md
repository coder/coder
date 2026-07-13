---
name: deep-review
description: "Multi-reviewer code review that cross-checks domain findings and posts one structured GitHub review."
---

# Deep Review

Run independent domain reviewers in parallel, cross-check their findings, then post one structured GitHub review.

## 0. Check proportionality

Use deep review for changes touching at least three subsystems, more than about 500 lines, or specialized risks such as security, concurrency, or databases.
If the PR has fewer than three files and fewer than 100 changed lines, and has no specialized risk, use [code-review](../../../.claude/skills/code-review/SKILL.md). For docs-only or
config-only PRs, use [doc-check](../../../.claude/skills/doc-check/SKILL.md).
Deep review requires parallel subagents.

Deep-review severity is P0-P4. Approximate code-review mapping: P0-P1 is red, P2 is yellow, and P3-P4 is blue.

## 1. Scope the change

Review every author with the same rigor. Create an output directory:

```sh
export REVIEW_DIR="/tmp/deep-review/$(date +%s)"
mkdir -p "$REVIEW_DIR"
```

Check for an earlier agent review:

```sh
gh pr view {number} --json reviews --jq '.reviews[] | select(.body | test("P[0-4]|\\*\\*Obs\\*\\*|\\*\\*Nit\\*\\*")) | .submittedAt' | head -1
```

For a re-review, read author replies, answer their questions, and inspect changes since the prior review. Write `$REVIEW_DIR/prior-findings.md` with columns for finding, author response, and
status. Classify each item as:

- **Resolved:** verify the fix and check for regressions.
- **Acknowledged:** the author agreed but deferred it.
- **Contested:** preserve the author's argument.
- **No response:** the author did not address it.

Carry only Contested and No response findings into the new review. Do not re-raise Resolved or Acknowledged findings.

Inspect the changed files and surrounding code. Record touched layers, intent, and domain risks. Check whether new files or abstractions duplicate established
patterns.

## 2. Select reviewer lanes

Always run these structural lanes:

| Lane | Focus |
| --- | --- |
| Test Auditor | Test authenticity, missing cases, readability |
| Edge Case Analyst | Edge cases, failure paths, hidden connections |
| Contract Auditor | Contract fidelity, lifecycle completeness, semantic honesty |

Add lanes when their domain is touched:

| Lane | Trigger and focus |
| --- | --- |
| Structural Analyst | API or type design, resource lifecycle, class-of-bug removal |
| Performance Analyst | Hot paths, loops, caches, allocation, resource exhaustion |
| Database Reviewer | Migrations, queries, schema, indexes, Go and SQL boundaries |
| Security Reviewer | Auth, endpoints, input handling, tokens, secrets |
| Product Reviewer | New features, config surfaces, over-engineering |
| Frontend Reviewer | UI state, render lifecycle, components, API shapes |
| Duplication Checker | New files, helpers, utilities, types, components |
| Go Architect | Go packages, API lifecycle, middleware, boundaries |
| Concurrency Reviewer | Goroutines, channels, locks, cancellation, shutdown |

Also run nit lanes:

- **Modernization Reviewer:** one per language with a non-empty filtered diff.
  - Go, `*.go`, read [the Go guide](../../../.claude/docs/GO.md).
  - TypeScript, `*.ts` and `*.tsx`, read [the TypeScript delta](references/typescript.md).
  - React, `*.tsx` and `*.jsx`, read [the React delta](references/react.md).
- **Style Reviewer:** `*.go`, `*.ts`, `*.tsx`, `*.py`, and `*.sh`.

Run both TypeScript and React lanes for `.tsx` changes.

## 3. Spawn reviewers in parallel

Each reviewer is read-only and fetches the PR or commit diff. It writes findings to `$REVIEW_DIR/{kebab-role}.md`. Name modernization outputs
`modernization-reviewer-{go|ts|react}.md`. Do not depend on subagent return text.

Use this structural prompt:

```text
Read AGENTS.md before starting.
You are the {Role Name} reviewer. Read
.agents/skills/deep-review/roles/{role-name}.md and
.agents/skills/deep-review/structural-reviewer-prompt.md.
Review: {PR number / branch / commit range}.
Output file: {REVIEW_DIR}/{role-name}.md
```

For nit lanes, replace `structural-reviewer-prompt.md` with
`nit-reviewer-prompt.md`, add the file filter, and add the language reference
from step 2 for modernization lanes.

For re-reviews, append:

```text
Read {REVIEW_DIR}/prior-findings.md. Do not re-raise Resolved or Acknowledged findings.
```

## 4. Cross-check and deduplicate

Read reviewer files one at a time to preserve attribution. Inventory each
finding's severity, location, summary, and exact evidence. Record missing or
failed lanes instead of silently dropping them.

Filter Tier 2 nits first. Drop subjective preferences, intentional choices, and
issues already enforced by tooling. Keep only practical improvements.

Cross-check structural findings for:

- **Contradictions:** preserve opposing recommendations and explain the conflict.
- **Interactions:** link findings that solve or worsen each other.
- **Convergence:** combine overlapping findings, trace their joint consequence,
  and reassess severity. Convergence is not a vote count.
- **Mechanism versus consequence:** restate the user, security, data, or
  regression impact before accepting severity.
- **Async chains:** trace the final visible or stateful effect, not only the
  missing cancellation or late callback.
- **Evidence:** downgrade or drop claims that are not demonstrated in the diff.
- **Novelty and duplication:** flag unnecessary new patterns or abstractions.
- **Scope:** downgrade redesign requests unrelated to the changed behavior.
- **Structural alternatives:** preserve designs that remove a documented
  tradeoff, even if the current code works.
- **Pre-existing behavior:** keep findings when new code now depends on or
  describes that behavior incorrectly.

Test every finding and observation for both upgrades and downgrades. Note a
severity spread greater than one level. A finding independently supported by at
least two reviewers needs a concrete counterargument before it is dropped.
Fold a nit into a structural finding when they target the same issue.

Use reviewer evidence as the source of record. Quote it exactly, then add
orchestrator framing and practical judgment. For convergence, start with the
sharpest evidence and add only unique evidence from up to two other reviewers.
Show each credited reviewer's severity. Verify every quote against the diff.

## 5. Post one GitHub review

Post a proper GitHub review with inline comments. Keep the body to two to four
sentences: summarize the change, mention concrete strong work when present, and count findings. For a
re-review, summarize what was addressed. Add this sentence for P0 or P1 items:
`This review contains findings that may need attention before merge.`

Inline format:

```text
**P{n}** One-sentence finding *(Reviewer Role P{n})*

> Exact reviewer evidence

Practical judgment, consequence, severity reasoning, and a focused fix.
```

Use `**Obs**` or `**Nit**` for those categories. Group co-located nits. For
convergent findings, credit the strongest two or three reviewers and preserve
individual severities.

Always submit event `COMMENT`. Never use `REQUEST_CHANGES` or `APPROVE`.
Save the diff to calculate diff-relative positions:

```sh
gh pr diff {number} > /tmp/pr.diff
```

Use REST explicitly. GitHub review comments use `position`, not `line`, `side`,
or `subject_type`. Pin file-level findings to position 1.

```sh
gh api -X POST \
  repos/{owner}/{repo}/pulls/{number}/reviews \
  --input review.json
```

```json
{
  "event": "COMMENT",
  "body": "Short summary and finding count.",
  "comments": [
    {"path": "file.go", "position": 42, "body": "**P1** Finding... *(Reviewer Role P1)*\n\n> Exact evidence\n\nPractical judgment."}
  ]
}
```

Be direct about correctness. Frame design alternatives as questions. Hedge
uncertainty, not demonstrated bugs. Follow the repository's
[pull-request conventions](../pull-requests/SKILL.md).

## 6. Follow up

Monitor author responses. Re-run from step 1 after a meaningful batch of fixes,
not after each reply. Preserve the prior-findings classification on every round.
