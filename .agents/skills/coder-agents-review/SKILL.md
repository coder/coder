---
name: coder-agents-review
description: "Run the Coder Agents Review loop on an open PR: trigger the GitHub app, fix feedback, and repeat until explicit approval."
---

# Coder Agents Review Loop

Drive an existing PR until `coder-agents-review` explicitly approves the current work.

## Completion contract

Stop only when all conditions hold:

- The latest relevant app response says `approved` on a complete status line, case-insensitively, or is an `APPROVED` review from the app.
- No actionable app thread remains unresolved, unless a reported policy or permission blocker prevents resolution.
- Relevant validation ran after the last change.
- The branch is pushed.

Never treat silence, old feedback, substring author matches, unpaginated
activity, or arbitrary uses of the word `approved` as approval.

## Configuration

- `PR_NUMBER`: infer from the current branch when unset.
- `REVIEW_TRIGGER`: exact body `/coder-agents-review`.
- `REVIEW_APP_LOGIN_REGEX`: `^coder-agents-review(\[bot\])?$`.
- `APPROVED_REGEX`: `^[[:space:]>]*approved[[:space:].!]*$`, applied
  case-insensitively to individual app-response lines. `not approved` and
  `cannot be approved yet` are not approvals.
- `LOCAL_VALIDATE_CMD`: repository validation command.
- `LOCAL_TEST_CMD`: optional targeted validation.
- `POLL_INTERVAL_SEC`: default `30`.
- `PAGE_SIZE`: default `100` per page, never a total-activity cap.

If observed app activity uses another login, discover the exact app login from
trusted GitHub activity or metadata. Match only that login. Do not guess.

## Inspect the PR

```bash
gh auth status
```

```bash
PR_NUMBER="${PR_NUMBER:-$(gh pr view --json number --jq .number)}"
echo "$PR_NUMBER"
```

```bash
gh pr view "$PR_NUMBER" --json number,title,url,headRefName,headRefOid,isDraft
```

```bash
OWNER="$(gh repo view --json owner --jq .owner.login)"
REPO="$(gh repo view --json name --jq .name)"
```

Inspect before posting. If app activity exists, handle it first. Post a trigger
only when there is no app response to handle and no pending trigger.

## Collect complete app activity

Fetch every page of top-level comments, reviews, review threads, and each
thread's nested comments before deriving state. Page each connection until
`pageInfo.hasNextPage` is false.

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

If nested thread comments have another page, fetch the thread by node ID and
page its comments before classifying resolution.

From the complete activity set, derive:

- Latest exact `/coder-agents-review` trigger.
- Latest app top-level comment and PR review.
- Latest app approval signal: `APPROVED` review or complete status line matching
  `APPROVED_REGEX`.
- Unresolved threads whose latest relevant comment came from the app.

Match authors only by `REVIEW_APP_LOGIN_REGEX` or the exact discovered login.

## Request and wait

For the first request, post only if no app activity and no unanswered trigger
exists:

```bash
gh pr comment "$PR_NUMBER" --body "/coder-agents-review"
```

If an unanswered trigger exists, do not duplicate it. If existing feedback is
actionable, fix it before requesting again. If an old approval cannot be tied
confidently to the current head, request a fresh review after pushing the
intended changes.

After every request, wait indefinitely for an app comment or review newer than
the trigger. A timeout is not a completion condition.

```bash
while :; do
  # refresh PR comments, reviews, and review threads
  # detect app response newer than the latest request
  # break only when the app has responded or a concrete blocker occurs
  sleep "${POLL_INTERVAL_SEC:-30}"
done
```

## Handle feedback

For each response:

1. Build a worklist from unresolved app threads and actionable top-level app
   comments.
2. Classify items as `fix-now`, `already-satisfied`, `blocked`, or
   `out-of-scope`.
3. Make the smallest safe in-scope fixes. Avoid unrelated cleanup.
4. Run validation, fix failures, and push.
5. Resolve addressed threads. If resolution is blocked, reply with a concise
   fix summary, leave the thread open, and report the blocker.
6. Post `/coder-agents-review` again and return to the wait loop.

Before every new request:

```bash
test -n "${LOCAL_VALIDATE_CMD:-}" && eval "$LOCAL_VALIDATE_CMD"
test -n "${LOCAL_TEST_CMD:-}" && eval "$LOCAL_TEST_CMD"
```

Resolve threads with repository helpers when available, otherwise use:

```bash
gh api graphql -f query='mutation($id: ID!) {
  resolveReviewThread(input: {threadId: $id}) {
    thread {
      isResolved
    }
  }
}' -F id="<thread_id>"
```

After every fix push, post the exact trigger again. Never merge or create a PR
unless the user explicitly asks.

## Approval and final report

A valid approval is an app review with state `APPROVED`, or a top-level app
comment containing a complete line matching `APPROVED_REGEX`. Split comment
bodies into lines. If the latest relevant app response is anything else, keep
iterating.

Report:

- PR number, URL, and current head SHA.
- Last trigger and app-response times.
- Exact approval evidence.
- Remaining unresolved app threads and blockers.
- Validation run after the final changes.

If forced to stop before approval, state the concrete blocker. Never claim the
loop succeeded.
