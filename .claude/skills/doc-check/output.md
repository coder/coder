# Output

How a doc-check run reports what it found.
Every run ends with a [run trace](#run-trace), whether or not it posts anything.
Findings use the [finding schema](./contract.md#finding-schema) from the sub-agent contract.

## Where findings go

| Class                                | Goes to                                                                     |
|--------------------------------------|-----------------------------------------------------------------------------|
| `wrong`, `missing`, `broken`, `rule` | An inline review comment on the line, and a checkbox in the summary comment |
| `suggestion`                         | An inline review comment on the line                                        |
| `friction`                           | The summary comment, under **Design notes**                                 |
| `adjacent`                           | The run trace only                                                          |
| Any class, `unverified`              | The summary comment, under **Incomplete**, as a count, and the run trace    |

A finding about a page the change does not touch cannot be an inline comment.
Put it in the summary comment.

## Posting in CI

Post nothing when the outcome is `clean`.
The run trace still records the clean result.

When the outcome is `incomplete`, post or update the summary comment even when no finding survived, so silence never reads as a clean review.

Before you post, read the PR head again with `gh pr view <PR> --repo coder/coder --json headRefOid`.
When it differs from the packet's head SHA, post nothing: a newer run reviews the new commits.
Record `head moved to <SHA>` in the trace under `not_done`.

When findings survive, post two things.

### 1. One review with inline comments

Post one PR review per round with the event `COMMENT`, never `APPROVE` or `REQUEST_CHANGES`.
Use `gh api -X POST repos/coder/coder/pulls/<PR>/reviews --input <file>` with the packet's head SHA as `commit_id`.

- One comment per finding, anchored to the line on the right side of the diff.
- Lead with the problem, then the evidence, then the fix.
- When the fix is local, end with a `suggestion` block that replaces exactly the anchored lines, with the file's own indentation.
- Mark blocking comments with `**Blocking:**` as the first word.
- Mention the persona name in parentheses after the problem, such as `(Fact Checker)`, so the author knows which lens raised it.

On a later round, comment only on lines the new commits changed, and on findings that are new.
Do not repeat a finding that already has an open thread.
Resolve a thread whose finding the new commits fixed, by replying `Fixed in <short SHA>.` and resolving it.

### 2. One summary comment

Search the PR for the marker `<!-- doc-check-sticky -->` and edit that comment if it exists.
Search again right before you post, because overlapping reviews of one PR can race.
Never post a second summary comment.

````markdown
## Documentation Check

### Blocking
- [ ] `docs/path/file.md` ([open](https://coder.com/docs/path/file)) - What needs to change
- [x] `docs/other/file.md` ([open](https://coder.com/docs/other/file)) - This was addressed
- ~~`docs/removed.md` - No longer needed~~ *(reverted in abc123)*

### New documentation needed
- [ ] `docs/suggested/path.md` - What to document, on a page that does not exist yet

### Design notes
- The setting has four names across the API, the UI, and the frontmatter, which makes `extending-agents.md:300` hard to keep accurate.

### Related open PRs
- #31147 would change the last sentence of the new section.

### Incomplete
- The Fact Checker did not finish, so claims on `docs/path/file.md` were not all checked against the code.
- 2 findings could not be verified and are not posted.

<details>
<summary>Findings for agents</summary>

```yaml
<the finding blocks for every inline comment and checkbox, in the finding schema>
```

</details>

---
*Automated review via [Coder Agents](https://coder.com/docs/ai-coder/agents). Reply to a finding's thread with the reason if the finding is wrong.*

<!-- doc-check-sticky -->
````

Include only the sections that apply.
**Related open PRs** lists each `related PR` entry from the Cartographer's `adjacent_gaps`.
The marker is the last line, every time.
Read the comment back after you post it, and add the marker if it is missing.

When a summary comment already exists, check off `[x]` items the new commits addressed, strike through items the change reverted, and add `[ ]` items for new blocking findings.
If nothing changed, leave the comment alone.

### Links

- Give the published URL: `https://coder.com/docs/` plus the path with the `docs/` prefix and the `.md` suffix removed.
- Write the path in backticks, then link it, so the item is searchable and one click opens it.
- A page the PR creates has no published URL yet. Name the path in backticks and say the page is new.
- A relative docs link 404s in a GitHub comment. Never use one.
- Link an anchor only when the heading exists on the base branch today.

### Wording

Write each comment so the agent or engineer who reads it can fix the problem without asking a question.
Name the sentence, the reason, and the replacement.
Do not hedge with "consider" or "might want to". A suggestion says what to change.
Comments follow the same style guide the docs do.

## Dry run output

A dry run posts nothing.
Its final message holds, in order:

1. Each inline comment you would have posted, as `file:line`, then the comment body.
2. The summary comment body you would have posted, or `No summary comment.`
3. The run trace.

## Run trace

End every run, in every mode, with this block as the last thing in your final message.
A later step reads it to set the commit status, so keep the field names exact.

```yaml
doc-check-trace:
  pr: 31159
  head_sha: 3580c80cc5fe8f4a665e8d606eea353296fdaa05
  policy_sha: 9666549f3c00460f8424e9fcb0d7772b9bbc254f   # the base commit the rules came from
  skill_sha: 9666549f3c00460f8424e9fcb0d7772b9bbc254f    # differs only in a dry run with a skill ref
  outcome: finding          # finding | clean | incomplete | error | skipped
  blocking: 3
  suggestions: 4
  rejected_by_verifier: 2
  unverified: 0
  max_parallel: 2
  agents:                   # every queued agent, with its status, or skipped with the reason
    - {id: change-model, status: complete}
    - {id: accuracy, status: complete, findings: 2}
    - {id: sweep-1, status: complete, findings: 1, rule_sections: 41/41}
    - {id: learner, status: skipped, reason: "No docs changed and no user-facing change."}
    - {id: verify-1, status: complete}
  rule_sections: {checked: 152, total: 152}
  deterministic_checks:
    - {name: make lint/emdash, result: pass}
    - {name: lint-docs (CI), result: success, annotations_on_changed_lines: 2}
  searches:                 # the Cartographer's old-literal searches
    - {query: "grep -rn 'model_invocable' docs/ | grep -v '^docs/reference/'", hits: 4}
  pr_claims_unsupported:
    - "Downloads add the line back for manual-only skills (true only for project skills)"
  related_open_prs: [31147]
  adjacent_gaps:
    - Workspace skills ignore disable-model-invocation, and no page tells Claude Code users.
  not_done:
    - lint (CI) was still running when the review finished.
```

`rule_sections` sums the sweeps: `checked` counts entries that are `pass`, `finding`, or `n/a`, and `total` counts the headings on the swept pages.

`outcome: incomplete` means the review ran but part of it did not finish, per step 7 of the workflow.
`outcome: error` means the review could not run.
Use it when the workspace is unusable, a required file cannot be read, or the diff cannot be fetched.
An `error` run posts nothing, and neither an `error` nor an `incomplete` run ever says or implies the docs are fine.
