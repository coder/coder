---
title: Git providers
---

Coder Agents leverages your existing [external authentication](../../admin/external-auth.md) configuration to power the in-chat diff viewer.
Self-hosted GitHub Enterprise deployments require one additional setting (`API_BASE_URL`) for this feature to work.
The diff viewer supports GitHub, GitLab, Gitea, and Forgejo.

## GitHub Enterprise configuration

For public `github.com`, no additional configuration is needed.

For self-hosted GitHub Enterprise, add `API_BASE_URL` to your [existing configuration](../../admin/external-auth.md#github-enterprise):

```dotenv
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

Without `API_BASE_URL`, Coder defaults to `https://api.github.com`.
Clone and push still work (they use `AUTH_URL` and `TOKEN_URL` directly), but the diff viewer silently fails because Coder builds its URL-matching patterns from the API base URL.

> [!NOTE]
> If you have both a `github.com` and a GitHub Enterprise (GHE) external auth config, only the GHE config needs `API_BASE_URL`.

## GitLab configuration

For `gitlab.com`, no additional `API_BASE_URL` is needed.
Coder automatically derives it from your `AUTH_URL` for self-hosted instances.

### Required scopes

The default GitLab scopes (`read_user`) are sufficient for basic authentication.
To use merge request features (diffs, status checks) with Coder Agents, configure:

```dotenv
CODER_EXTERNAL_AUTH_0_ID="primary-gitlab"
CODER_EXTERNAL_AUTH_0_TYPE=gitlab
CODER_EXTERNAL_AUTH_0_CLIENT_ID=xxxxxx
CODER_EXTERNAL_AUTH_0_CLIENT_SECRET=xxxxxxx
CODER_EXTERNAL_AUTH_0_SCOPES="write_repository read_api"
```

The `read_api` scope grants read access to the API (needed for fetching merge request metadata and diffs).
The `write_repository` scope allows pushing commits and creating merge requests.

### Self-hosted GitLab

For self-hosted GitLab, set `AUTH_URL` and `TOKEN_URL` to your instance.
Coder derives `API_BASE_URL` automatically from `AUTH_URL`:

```dotenv
CODER_EXTERNAL_AUTH_0_ID="primary-gitlab"
CODER_EXTERNAL_AUTH_0_TYPE=gitlab
CODER_EXTERNAL_AUTH_0_CLIENT_ID=xxxxxx
CODER_EXTERNAL_AUTH_0_CLIENT_SECRET=xxxxxxx
CODER_EXTERNAL_AUTH_0_AUTH_URL="https://gitlab.example.com/oauth/authorize"
CODER_EXTERNAL_AUTH_0_TOKEN_URL="https://gitlab.example.com/oauth/token"
CODER_EXTERNAL_AUTH_0_SCOPES="write_repository read_api"
CODER_EXTERNAL_AUTH_0_REGEX=gitlab\.example\.com
```

> [!NOTE]
> You can also set `API_BASE_URL` explicitly if needed (for example, `https://gitlab.example.com/api/v4`), but this is usually unnecessary.

## Gitea and Forgejo configuration

Forgejo uses the `gitea` external auth type.
Create the OAuth application as described in [Gitea](../../admin/external-auth.md#gitea).
For a self-hosted instance, set `AUTH_URL` to your instance.
Coder derives the token, validation, and API URLs from it.
For Forgejo, set `DISPLAY_NAME` so users see Forgejo instead of Gitea on the link button:

```dotenv
CODER_EXTERNAL_AUTH_0_ID="gitea"
CODER_EXTERNAL_AUTH_0_TYPE=gitea
CODER_EXTERNAL_AUTH_0_CLIENT_ID=xxxxxx
CODER_EXTERNAL_AUTH_0_CLIENT_SECRET=xxxxxxx
CODER_EXTERNAL_AUTH_0_AUTH_URL="https://git.example.com/login/oauth/authorize"
CODER_EXTERNAL_AUTH_0_DISPLAY_NAME="Forgejo"
```

The default scopes (`read:repository write:repository read:user`) cover clone, push, and the pull request data the diff viewer reads.

If the instance is served under a subpath, such as `https://example.com/git`, set `TOKEN_URL`, `VALIDATE_URL`, and `API_BASE_URL` to full URLs, for example `https://example.com/git/login/oauth/access_token`, `https://example.com/git/login/oauth/userinfo`, and `https://example.com/git/api/v1`.
Coder builds pull request and repository URLs from it.

## Known limitations

### GitLab

The GitLab provider has some semantic differences compared to the GitHub provider:

- **Approved** uses GitLab's threshold-based approval (for example, "all required approvals met") rather than GitHub's "at least one approval and no changes requested" model.
- **Changes requested** has no GitLab equivalent.
  This field is always reported as `false`.
- **Reviewer count** only counts users who have approved, not all assigned reviewers.

These gaps are tracked internally and might be refined in future releases.

### Gitea and Forgejo

- **Branch diffs** before a pull request exists require Gitea 1.27 or later.
  Forgejo and earlier Gitea releases can't return a branch diff through the API.
  Until a pull request is opened, the diff viewer shows an error that the server can't diff a branch without a pull request.
- **Branch to pull request lookup** runs as a server-side search on Forgejo.
  Gitea doesn't support that search, so Coder scans up to 500 open pull requests in the repository to find a match.
  In a Gitea repository with more open pull requests, the pull request for a branch might not be found.
- **Approved** and **Changes requested** use each reviewer's latest review, as on GitHub.
  Approvals that Gitea marks as stale after new commits still count unless the review was dismissed.
- **Refresh cost**: before a pull request is found, each refresh searches the repository's open pull requests, which takes up to 10 requests on Gitea.
  Afterward, each refresh makes three API requests (pull request, commit count, and reviews).
  Unlike the GitHub provider, responses aren't cached with conditional requests.
- **Token revocation**: Gitea and Forgejo have no endpoint to revoke OAuth tokens, so unlinking the external auth account in Coder doesn't revoke the token on the server.
  Revoke it from the user's **Settings** > **Applications** page on the Gitea or Forgejo instance.

## Troubleshooting

### Diffs not appearing on GHE

Add `API_BASE_URL` to your GHE external auth config and restart Coder.
Diffs should appear within a couple of minutes.

### Users not seeing diffs

The chat owner must have linked their account through the relevant external auth provider.

<a id="checking-logs"></a>

### Check logs

Look for gitsync warnings such as `no provider for origin` or `resolve token` errors.
