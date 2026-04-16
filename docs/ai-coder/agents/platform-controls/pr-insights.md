# PR Insights

PR Insights tracks pull requests created by Coder Agents and surfaces
analytics on PR activity, merge rates, and cost efficiency.

## How it works

A background worker monitors active agent chats for git activity. When an
agent pushes a branch or creates a pull request, the worker resolves the git
remote origin against configured external auth providers.

The worker uses the matched provider's API to fetch PR metadata: status, diff
stats, review state, and merge outcome.

> [!NOTE]
> Only `github` type external auth providers are supported for PR Insights
> today.

## Requirements

For PR data to appear in analytics, all of the following must be true:

1. **External auth is configured for your git host** — The external auth
   config must have `type` set to `github` with a regex matching your
   repository URLs. See
   [External Authentication](../../../admin/external-auth/index.md).

1. **Users have linked their external auth** — The user who ran the agent
   task must have authenticated with the relevant external auth provider.
   Without a linked token, the worker cannot fetch PR data and retries on a
   backoff schedule.

1. **The agent reported a git reference** — The agent must push to a branch
   with a configured remote origin. If no branch or remote origin is
   reported, the worker skips the chat.

## GitHub Enterprise configuration

For self-hosted GitHub Enterprise (GHE) deployments, you must set
`CODER_EXTERNAL_AUTH_X_API_BASE_URL` in addition to the standard external
auth variables.

Without this, Coder defaults to `https://api.github.com` as the API
endpoint. Auth operations like clone and push still work because they use the
`AUTH_URL` and `TOKEN_URL` you configured. However, PR Insights builds its
URL-matching patterns from the API base URL. When the default points to
`github.com`, the worker cannot match PR URLs or repository origins from your
GHE instance, and PR data silently fails to appear.

Example configuration for GHE:

```env
CODER_EXTERNAL_AUTH_0_ID="primary-github"
CODER_EXTERNAL_AUTH_0_TYPE=github
CODER_EXTERNAL_AUTH_0_CLIENT_ID=xxxxxx
CODER_EXTERNAL_AUTH_0_CLIENT_SECRET=xxxxxxx
CODER_EXTERNAL_AUTH_0_AUTH_URL="https://github.example.com/login/oauth/authorize"
CODER_EXTERNAL_AUTH_0_TOKEN_URL="https://github.example.com/login/oauth/access_token"
CODER_EXTERNAL_AUTH_0_VALIDATE_URL="https://github.example.com/api/v3/user"
CODER_EXTERNAL_AUTH_0_API_BASE_URL="https://github.example.com/api/v3"
CODER_EXTERNAL_AUTH_0_REGEX=github\.example\.com
```

> [!NOTE]
> Public `github.com` configurations do not need `API_BASE_URL` — the
> default (`https://api.github.com`) is already correct.

## Dashboard

Navigate to **Agents** > **Insights** > **PR Insights**.

### Summary view

Period-over-period comparison cards:

- **Total PRs created** — count of PRs opened during the selected period.
- **Total PRs merged** — count of PRs merged during the selected period.
- **Merge rate** — percentage of created PRs that were merged.
- **Cost per merged PR** — total agent spend divided by merged PR count.
- **Approval rate** — percentage of PRs that received an approving review.

### Per-model breakdown

Table showing per-model stats:

| Column           | Description                                   |
|------------------|-----------------------------------------------|
| Model            | LLM model used by the agent                   |
| Total PRs        | Number of PRs created using this model        |
| Merge rate       | Percentage of PRs merged for this model       |
| Cost per merged  | Average agent cost per merged PR              |
| Additions        | Total lines added across PRs for this model   |
| Deletions        | Total lines removed across PRs for this model |

### Recent PRs

Table of individual PRs:

| Column        | Description                                    |
|---------------|------------------------------------------------|
| PR            | PR number with link to the pull request        |
| State         | Current state: open, merged, or closed         |
| Additions     | Lines added                                    |
| Deletions     | Lines removed                                  |
| Review status | Approval state from reviewers                  |
| Chat          | Link to the associated agent chat              |
| Cost          | Agent spend attributed to this PR              |

### Time series

Daily chart of PRs created, merged, and closed over the selected period.

## Troubleshooting

### PRs not appearing

Check that `API_BASE_URL` is set for GHE deployments. Verify the user has
linked their external auth. Check Coder logs for gitsync warnings like
`no provider for origin` or token resolution errors.

### Only github.com PRs appear

If you have multiple external auth configs (e.g., `github.com` + GHE),
ensure the GHE config has `API_BASE_URL` set. The `github.com` config works
without it because the default is already correct.

### PR data delayed

The gitsync worker polls on a ~10 second interval. New PRs typically appear
within a couple of minutes. If a token refresh fails, the worker backs off
for 10 minutes before retrying.
