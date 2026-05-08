---
name: coder-agents-review
description: "Use this skill when a repository already has an open pull request and you need to run the Coder Agents Review loop: request review with `/coder-agents-review` when needed, wait for feedback from the `coder-agents-review` GitHub app, fix issues, and repeat until the app comments `approved`."
---

# Coder Agents Review Loop

## Goal

Drive an existing pull request until the GitHub app `coder-agents-review`
has approved the current work.

The loop is:

1. if the PR has no existing `coder-agents-review` review or comment,
   post `/coder-agents-review`
2. wait for `coder-agents-review` to respond
3. fix actionable issues with the smallest safe diff
4. validate and push
5. request another review with `/coder-agents-review`
6. repeat until the app comments `approved`

## Definition of done

Only stop when all of these are true:

- the latest `coder-agents-review` response for the current work says
  `approved` (case-insensitive), or is a GitHub `APPROVED` review from
  that app
- there are no unresolved actionable `coder-agents-review` review threads
  left from the latest feedback, unless a policy or permission blocker
  prevents resolution and you reported it
- local validation relevant to the touched code has been run after the
  last changes
- the branch has been pushed

If you stop early, say exactly why.

## Non-negotiable behavior

- Inspect the PR before posting anything.
- If the PR has no review or comment from `coder-agents-review`, post a
  top-level PR comment with the exact body `/coder-agents-review`.
- If `coder-agents-review` activity is already present, start from that
  feedback instead of posting a duplicate trigger immediately.
- After every fix push, post `/coder-agents-review` again.
- Wait indefinitely for the app's first response after each request. Do
  not treat silence as approval.
- Fix the app's actionable feedback with the smallest reasonable diff.
  Avoid unrelated cleanup.
- Resolve addressed app review threads if you can. If you cannot, reply
  with a short fix summary and report the blocker.
- Never create or merge a PR unless the user explicitly asks.

## Disclosure rule

The trigger comment should stay exactly `/coder-agents-review` so the app
can process it reliably.

If you need to disclose that Mux is acting on Mike's behalf, use a short
separate comment only when that disclosure is not already present on the
PR and only if adding extra text to the slash-command comment could break
the trigger.

Use this exact disclosure text:

```text
> Mux is acting on Mike's behalf.
```

## Defaults and config

Use repository conventions first. Otherwise use these defaults.

- `PR_NUMBER`: PR number to operate on. If unset, infer it from the
  current branch's open PR.
- `REVIEW_TRIGGER`: exact request comment.

  ```text
  /coder-agents-review
  ```

- `REVIEW_APP_LOGIN_REGEX`: default match for the app author login.

  ```text
  ^coder-agents-review(\[bot\])?$
  ```

- `REVIEW_APP_SLUG`: fallback case-insensitive substring match when the
  login format differs.

  ```text
  coder-agents-review
  ```

- `APPROVED_REGEX`: case-insensitive approval match.

  ```text
  (^|[^[:alpha:]])approved([^[:alpha:]]|$)
  ```

- `LOCAL_VALIDATE_CMD`: repo-standard validation command.
- `LOCAL_TEST_CMD`: optional targeted validation for the touched area.
- `POLL_INTERVAL_SEC`: default `30`.
- `PAGE_SIZE`: default `100`. Use it for each GitHub pagination
  request, not as a cap on the total activity fetched.

If the app login does not match the default regex, discover the real
login from existing PR activity and continue with that exact login. Do
not guess when the evidence is unclear.

## Discover PR context

Confirm GitHub auth:

```bash
gh auth status
```

Infer the PR number if needed:

```bash
PR_NUMBER="${PR_NUMBER:-$(gh pr view --json number --jq .number)}"
echo "$PR_NUMBER"
```

Get basic PR info:

```bash
gh pr view "$PR_NUMBER" --json number,title,url,headRefName,headRefOid,isDraft
```

Identify owner and repo:

```bash
OWNER="$(gh repo view --json owner --jq .owner.login)"
REPO="$(gh repo view --json name --jq .name)"
```

## Collect app activity

Inspect top-level PR comments, PR reviews, and review threads. Fetch all
pages before deriving review state. GitHub GraphQL connections are
paginated, so a single `first:100` request can miss newer review-app
activity on busy PRs.

Page these connections until `pageInfo.hasNextPage` is false:

- `comments`, for top-level PR comments
- `reviews`, for PR reviews
- `reviewThreads`, for review thread metadata
- each review thread's `comments`, when its nested comment connection has
  more pages

Example page query:

```bash
gh api graphql -f query='query(
  $owner: String!
  $repo: String!
  $number: Int!
  $pageSize: Int!
  $commentsAfter: String
  $reviewsAfter: String
  $threadsAfter: String
) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $number) {
      number
      url
      headRefName
      headRefOid
      comments(first: $pageSize, after: $commentsAfter) {
        pageInfo { hasNextPage endCursor }
        nodes {
          body
          createdAt
          url
          author { login }
        }
      }
      reviews(first: $pageSize, after: $reviewsAfter) {
        pageInfo { hasNextPage endCursor }
        nodes {
          body
          state
          submittedAt
          url
          author { login }
          commit { oid }
        }
      }
      reviewThreads(first: $pageSize, after: $threadsAfter) {
        pageInfo { hasNextPage endCursor }
        nodes {
          id
          isResolved
          comments(first: $pageSize) {
            pageInfo { hasNextPage endCursor }
            nodes {
              body
              createdAt
              url
              author { login }
            }
          }
        }
      }
    }
  }
}' \
-F owner="$OWNER" \
-F repo="$REPO" \
-F number="$PR_NUMBER" \
-F pageSize="${PAGE_SIZE:-100}"
```

If a review thread's nested `comments.pageInfo.hasNextPage` is true,
fetch that thread by node ID and keep paging its comments before using
that thread to decide whether feedback remains unresolved.

Build these facts from the complete paginated activity set:

- latest exact trigger comment with body `/coder-agents-review`
- latest top-level comment from the review app
- latest PR review from the review app
- latest review-app approval signal, either a review with state
  `APPROVED` or a comment body matching `APPROVED_REGEX`
- unresolved review threads where the latest relevant comment came from
  the review app

Treat the app as matched when the author login matches
`REVIEW_APP_LOGIN_REGEX`, or when the login contains
`REVIEW_APP_SLUG` case-insensitively and the surrounding evidence makes
it clear that it is the review app.

## Request rules

### First request

If the PR has no review or comment from `coder-agents-review`, post the
exact trigger comment:

```bash
gh pr comment "$PR_NUMBER" --body "/coder-agents-review"
```

If needed, add the disclosure comment right after it.

### Existing activity already present

If the PR already has `coder-agents-review` activity, do not post another
trigger immediately just because the skill started.

Instead:

1. inspect the latest app feedback
2. if the latest app response is already an approval for the current work,
   finish
3. if the latest app response contains actionable feedback, fix that
   feedback first
4. after pushing fixes, post `/coder-agents-review` again

If you cannot confidently tell whether an old approval covers the current
head SHA, do not guess. Push the intended fixes, then request a fresh
review.

## Wait loop

After every review request, wait until the app responds. Keep polling. Do
not replace waiting with a timeout.

A minimal loop is:

```bash
while :; do
  # refresh PR comments, reviews, and review threads
  # detect app response newer than the latest request
  # break only when the app has responded or a concrete blocker occurs
  sleep "${POLL_INTERVAL_SEC:-30}"
done
```

A response counts when a new `coder-agents-review` comment or review is
visible after the latest trigger comment.

## Handling feedback

When the app leaves feedback:

1. build a worklist from unresolved app review threads and any actionable
   top-level app comments
2. classify each item as `fix-now`, `already-satisfied`, `blocked`, or
   `out-of-scope`
3. implement the smallest safe in-scope fixes
4. run local validation
5. push the branch
6. resolve the threads you actually fixed, or reply with a concise summary
   if resolution is blocked
7. post `/coder-agents-review` again
8. return to the wait loop

Do not widen scope for opportunistic cleanup.

## Validation

Before every new review request:

1. run the repository's standard validation command, if available
2. run targeted tests for the touched area, if appropriate
3. fix failures before pushing

Examples:

```bash
test -n "${LOCAL_VALIDATE_CMD:-}" && eval "$LOCAL_VALIDATE_CMD"
test -n "${LOCAL_TEST_CMD:-}" && eval "$LOCAL_TEST_CMD"
```

Do not claim success if code changed but relevant validation did not run.

## Resolving review threads

Prefer repository helpers if they exist. Otherwise resolve threads with
GitHub GraphQL:

```bash
gh api graphql -f query='mutation($id: ID!) {
  resolveReviewThread(input: {threadId: $id}) {
    thread {
      isResolved
    }
  }
}' -F id="<thread_id>"
```

If you cannot resolve a fixed thread yourself:

- leave a concise reply describing the fix
- keep the thread open
- report the blocker in the final summary

## Completion rule

Only finish when the latest relevant app response is an approval for the
current work.

A valid approval is either:

- a review from the app with state `APPROVED`, or
- a top-level app comment whose body matches `APPROVED_REGEX`

If the latest app response is anything else, keep iterating.

## Final report

When the loop finishes, report:

- PR number and URL
- current head SHA
- when `/coder-agents-review` was last requested
- when `coder-agents-review` last responded
- the approval evidence, review state or matching comment text
- whether any app threads remain unresolved, and why
- what validation was run
- any blockers if the loop ended early

## Operating rules

- Never post duplicate trigger comments on the same head when the app is
  already reviewing or has already left feedback you have not handled yet.
- Never treat silence as approval.
- Never claim success without explicit app approval evidence.
- Never ignore unresolved actionable app feedback.
- Never skip validation after making changes.
- Never derive approval or completion from unpaginated PR activity.
- Prefer `gh` and repo-native helpers over manual browser work.
