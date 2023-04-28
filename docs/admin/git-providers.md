# Git Providers

Coder integrates with git providers so developers can authenticate with repositories within their workspace.

## How it works

When developers use `git` inside their workspace, they are first prompted to authenticate. After that, Coder will store/refresh tokens for future operations.

<video autoplay playsinline loop>
  <source src="https://github.com/coder/coder/blob/main/site/static/gitauth.mp4?raw=true" type="video/mp4">
Your browser does not support the video tag.
</video>

## Configuration

To add a git provider, you'll need to create an OAuth application. The following providers are supported:

- [GitHub](https://docs.github.com/en/developers/apps/building-oauth-apps/creating-an-oauth-app) (GitHub apps are also supported)
- [GitLab](https://docs.gitlab.com/ee/integration/oauth_provider.html)
- [BitBucket](https://support.atlassian.com/bitbucket-cloud/docs/use-oauth-on-bitbucket-cloud/)
- [Azure DevOps](https://learn.microsoft.com/en-us/azure/devops/integrate/get-started/authentication/oauth?view=azure-devops)

Example callback URL: `https://coder.example.com/gitauth/primary-github/callback`. Use an arbitrary ID for your provider (e.g. `primary-github`).

Set the following environment variables to [configure the Coder server](./configure.md):

```console
CODER_GITAUTH_0_ID="primary-github"
CODER_GITAUTH_0_TYPE=github|gitlab|azure-devops|bitbucket
CODER_GITAUTH_0_CLIENT_ID=xxxxxx
CODER_GITAUTH_0_CLIENT_SECRET=xxxxxxx
```

### GitHub Enterprise

GitHub Enterprise requires the following authentication and token URLs:

```console
CODER_GITAUTH_0_VALIDATE_URL="https://github.example.com/login/oauth/access_token/info"
CODER_GITAUTH_0_AUTH_URL="https://github.example.com/login/oauth/authorize"
CODER_GITAUTH_0_TOKEN_URL="https://github.example.com/login/oauth/access_token"
```

### Azure DevOps

Azure DevOps requires the following environment variables:

```console
CODER_GITAUTH_0_ID="primary-azure-devops"
CODER_GITAUTH_0_TYPE=azure-devops
CODER_GITAUTH_0_CLIENT_ID=xxxxxx
# Ensure this value is your "Client Secret", not "App Secret"
CODER_GITAUTH_0_CLIENT_SECRET=xxxxxxx
CODER_GITAUTH_0_AUTH_URL="https://app.vssps.visualstudio.com/oauth2/authorize"
CODER_GITAUTH_0_TOKEN_URL="https://app.vssps.visualstudio.com/oauth2/token"
```

### Self-managed git providers

Custom authentication and token URLs should be
used for self-managed Git provider deployments.

```console
CODER_GITAUTH_0_AUTH_URL="https://github.example.com/oauth/authorize"
CODER_GITAUTH_0_TOKEN_URL="https://github.example.com/oauth/token"
CODER_GITAUTH_0_VALIDATE_URL="https://your-domain.com/oauth/token/info"
```

### Custom scopes

Optionally, you can request custom scopes:

```console
CODER_GITAUTH_0_SCOPES="repo:read repo:write write:gpg_key"
```

### Multiple git providers (enterprise)

Multiple providers are an Enterprise feature. [Learn more](../enterprise.md).

A custom regex can be used to match a specific repository or organization to limit auth scope. Here's a sample config:

```console
# Provider 1) github.com
CODER_GITAUTH_0_ID=primary-github
CODER_GITAUTH_0_TYPE=github
CODER_GITAUTH_0_CLIENT_ID=xxxxxx
CODER_GITAUTH_0_CLIENT_SECRET=xxxxxxx
CODER_GITAUTH_0_REGEX=github.com/orgname

# Provider 2) github.example.com
CODER_GITAUTH_1_ID=secondary-github
CODER_GITAUTH_1_TYPE=github
CODER_GITAUTH_1_CLIENT_ID=xxxxxx
CODER_GITAUTH_1_CLIENT_SECRET=xxxxxxx
CODER_GITAUTH_1_REGEX=github.example.com
CODER_GITAUTH_1_AUTH_URL="https://github.example.com/login/oauth/authorize"
CODER_GITAUTH_1_TOKEN_URL="https://github.example.com/login/oauth/access_token"
CODER_GITAUTH_1_VALIDATE_URL="https://github.example.com/login/oauth/access_token/info"
```

To support regex matching for paths (e.g. github.com/orgname), you'll need to add this to the [Coder agent startup script](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/agent#startup_script):

```console
git config --global credential.useHttpPath true
```

## Require git authentication in templates

If your template requires git authentication (e.g. running `git clone` in the [startup_script](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/agent#startup_script)), you can require users authenticate via git prior to creating a workspace:

![Git authentication in template](../images/admin/git-auth-template.png)

The following example will require users authenticate via GitHub and auto-clone a repo
into the `~/coder` directory.

```hcl
data "coder_git_auth" "github" {
  # Matches the ID of the git auth provider in Coder.
  id = "github"
}

resource "coder_agent" "dev" {
  os   = "linux"
  arch = "amd64"
  dir  = "~/coder"
  env = {
    GITHUB_TOKEN : data.coder_git_auth.github.access_token
  }
  startup_script = <<EOF
if [ ! -d ~/coder ]; then
    git clone https://github.com/coder/coder
fi
EOF
}
```

See the [Terraform provider documentation](https://registry.terraform.io/providers/coder/coder/latest/docs/data-sources/git_auth) for all available options.
