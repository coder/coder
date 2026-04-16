# Git Providers

Coder Agents leverages your existing
[external authentication](../../../admin/external-auth/index.md) configuration
to power the in-chat diff viewer and [PR Insights](./pr-insights.md).
Self-hosted GitHub Enterprise deployments require one additional setting
(`API_BASE_URL`) for these features to work.

> [!NOTE]
> Only `github` type external auth providers are supported today.

## GitHub Enterprise configuration

For public `github.com`, no additional configuration is needed.

For self-hosted GitHub Enterprise, add `API_BASE_URL` to your
[existing configuration](../../../admin/external-auth/index.md#github-enterprise):

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

Without `API_BASE_URL`, Coder defaults to `https://api.github.com`. Clone
and push still work (they use `AUTH_URL` and `TOKEN_URL` directly), but
the diff viewer and PR Insights silently fail because Coder builds its
URL-matching patterns from the API base URL.

> [!NOTE]
> If you have both a `github.com` and a GHE external auth config, only the
> GHE config needs `API_BASE_URL`.

## Troubleshooting

### Diffs or PR data not appearing on GHE

Add `API_BASE_URL` to your GHE external auth config and restart Coder.
Data should appear within a couple of minutes.

### Users not seeing diffs

The chat owner must have linked their account through the relevant external
auth provider.

### Checking logs

Look for gitsync warnings such as `no provider for origin` or
`resolve token` errors.
