# PR Insights

PR Insights tracks pull requests created by Coder Agents and surfaces
analytics on PR activity, merge rates, and cost efficiency. The dashboard
(under **Agents** > **Insights** > **PR Insights**) shows merge rates,
cost per merged PR, per-model breakdowns, and individual PR status.

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

For self-hosted GitHub Enterprise deployments, additional configuration is
required. See [Git Providers](./git-providers.md#github-enterprise-configuration).

## Troubleshooting

### PRs not appearing

Verify the user has linked their external auth. Check Coder logs for gitsync
warnings like `no provider for origin` or token resolution errors. For GitHub
Enterprise, confirm that `API_BASE_URL` is set — see
[Git Providers](./git-providers.md#troubleshooting).

### Only github.com PRs appear

If you have multiple external auth configs (e.g., `github.com` + GHE),
ensure the GHE config has `API_BASE_URL` set. The `github.com` config works
without it because the default is already correct.

### PR data delayed

The background worker polls on a ~10 second interval. New PRs typically
appear within a couple of minutes. If a token refresh fails, the worker
backs off for 10 minutes before retrying.
