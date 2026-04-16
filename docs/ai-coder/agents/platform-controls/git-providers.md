# Git Providers

Coder Agents uses your existing
[external authentication](../../../admin/external-auth/index.md) configuration
to interact with git hosting providers. External auth handles clone, push, and
credential flows. The git provider integration documented here extends that
foundation to support Coder Agents features that need richer access to your
git host's API — specifically the in-chat diff viewer and
[PR Insights](./pr-insights.md).

> [!NOTE]
> Only `github` type external auth providers are supported for git provider
> features today.

## What the git provider enables

When a `github` type external auth provider is fully configured, Coder Agents
can:

- **Show diffs in the chat UI** — the agent resolves the working branch and
  remote origin, then fetches the diff from the GitHub API and displays it
  inline in the conversation.
- **Track PR analytics** — a background worker fetches PR metadata (status,
  review state, merge outcome, diff stats) and surfaces it in the
  [PR Insights](./pr-insights.md) dashboard.

Both features rely on the same underlying mechanism: Coder resolves the git
remote origin against your external auth configs, builds host-specific URL
patterns from the provider's API base URL, and uses the matched provider to
make API calls.

## GitHub Enterprise configuration

For public `github.com`, the default configuration from
[external authentication](../../../admin/external-auth/index.md) is
sufficient — no additional settings are needed.

For self-hosted GitHub Enterprise (GHE) deployments, you must set
`CODER_EXTERNAL_AUTH_X_API_BASE_URL` in addition to the standard external
auth variables documented in the
[GitHub Enterprise](../../../admin/external-auth/index.md#github-enterprise)
section.

Without `API_BASE_URL`, Coder defaults to `https://api.github.com`. Standard
auth operations (clone, push, token refresh) still work because they use
`AUTH_URL` and `TOKEN_URL` directly. However, the git provider builds its
URL-matching patterns from the API base URL. When the default points to
`github.com`, Coder cannot match repository origins or PR URLs from your GHE
instance, and both the diff viewer and PR Insights silently return no data.

### Example

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

The key addition compared to the base
[GitHub Enterprise](../../../admin/external-auth/index.md#github-enterprise)
configuration is `API_BASE_URL`. This tells Coder to build URL patterns for
your GHE host and to direct API calls there when fetching diffs and PR
metadata.

> [!NOTE]
> If you have both a `github.com` and a GHE external auth config, only the
> GHE config needs `API_BASE_URL`. The `github.com` default is already
> correct.

## Troubleshooting

### Diffs and PR data not appearing on GHE

The most common cause is a missing `API_BASE_URL`. Add it to your GHE
external auth config and restart Coder. The diff viewer and PR Insights
should start working within a couple of minutes.

### Users not seeing diffs

The user who owns the chat must have linked their account through the
relevant external auth provider. Without a linked token, Coder cannot
make API calls on their behalf.

### Checking logs

Look for gitsync warnings in Coder logs such as `no provider for origin` or
`resolve token` errors. These indicate that the remote origin did not match
any configured external auth provider, or that no valid token was available.
