# External Authentication

Coder integrates with Git and OpenID Connect to automate away the need for
developers to authenticate with external services within their workspace.

## Git Providers

When developers use `git` inside their workspace, they are prompted to
authenticate. After that, Coder will store and refresh tokens for future
operations.

<video autoplay playsinline loop>
  <source src="https://github.com/coder/coder/blob/main/site/static/external-auth.mp4?raw=true" type="video/mp4">
Your browser does not support the video tag.
</video>

## Configuration

To add an external authentication provider, you'll need to create an OAuth
application. The following providers are supported:

- [GitHub](#github)
- [GitLab](https://docs.gitlab.com/ee/integration/oauth_provider.html)
- [BitBucket](https://support.atlassian.com/bitbucket-cloud/docs/use-oauth-on-bitbucket-cloud/)
- [Azure DevOps](https://learn.microsoft.com/en-us/azure/devops/integrate/get-started/authentication/oauth?view=azure-devops)

Example callback URL:
`https://coder.example.com/external-auth/primary-github/callback`. Use an
arbitrary ID for your provider (e.g. `primary-github`).

Set the following environment variables to
[configure the Coder server](./configure.md):

```env
CODER_EXTERNAL_AUTH_0_ID="primary-github"
CODER_EXTERNAL_AUTH_0_TYPE=github|gitlab|azure-devops|bitbucket-cloud|bitbucket-server|<name of service e.g. jfrog>
CODER_EXTERNAL_AUTH_0_CLIENT_ID=xxxxxx
CODER_EXTERNAL_AUTH_0_CLIENT_SECRET=xxxxxxx

# Optionally, configure a custom display name and icon
CODER_EXTERNAL_AUTH_0_DISPLAY_NAME="Google Calendar"
CODER_EXTERNAL_AUTH_0_DISPLAY_ICON="https://mycustomicon.com/google.svg"
```

### GitHub

1. [Create a GitHub App](https://docs.github.com/en/apps/creating-github-apps/registering-a-github-app/registering-a-github-app)
   to enable fine-grained access to specific repositories, or a subset of
   permissions for security.

   ![Register GitHub App](../images/admin/github-app-register.png)

2. Adjust the GitHub App permissions. You can use more or less permissions than
   are listed here, this is merely a suggestion that allows users to clone
   repositories:

   ![Adjust GitHub App Permissions](../images/admin/github-app-permissions.png)

   | Name          | Permission   | Description                                            |
   | ------------- | ------------ | ------------------------------------------------------ |
   | Contents      | Read & Write | Grants access to code and commit statuses.             |
   | Pull requests | Read & Write | Grants access to create and update pull requests.      |
   | Workflows     | Read & Write | Grants access to update files in `.github/workflows/`. |
   | Metadata      | Read-only    | Grants access to metadata written by GitHub Apps.      |

3. Install the App for your organization. You may select a subset of
   repositories to grant access to.

   ![Install GitHub App](../images/admin/github-app-install.png)

### GitHub Enterprise

GitHub Enterprise requires the following environment variables:

```env
CODER_EXTERNAL_AUTH_0_ID="primary-github"
CODER_EXTERNAL_AUTH_0_TYPE=github-enterprise
CODER_EXTERNAL_AUTH_0_CLIENT_ID=xxxxxx
CODER_EXTERNAL_AUTH_0_CLIENT_SECRET=xxxxxxx
CODER_EXTERNAL_AUTH_0_VALIDATE_URL="https://github.example.com/api/v3/user"
CODER_EXTERNAL_AUTH_0_AUTH_URL="https://github.example.com/login/oauth/authorize"
CODER_EXTERNAL_AUTH_0_TOKEN_URL="https://github.example.com/login/oauth/access_token"
```

### Bitbucket Server

Bitbucket Server requires the following environment variables:

```env
CODER_EXTERNAL_AUTH_0_TYPE="bitbucket-server"
CODER_EXTERNAL_AUTH_0_ID=bitbucket
CODER_EXTERNAL_AUTH_0_CLIENT_ID=xxx
CODER_EXTERNAL_AUTH_0_CLIENT_SECRET=xxx
CODER_EXTERNAL_AUTH_0_AUTH_URL=https://bitbucket.domain.com/rest/oauth2/latest/authorize
```

### Azure DevOps

Azure DevOps requires the following environment variables:

```env
CODER_EXTERNAL_AUTH_0_ID="primary-azure-devops"
CODER_EXTERNAL_AUTH_0_TYPE=azure-devops
CODER_EXTERNAL_AUTH_0_CLIENT_ID=xxxxxx
# Ensure this value is your "Client Secret", not "App Secret"
CODER_EXTERNAL_AUTH_0_CLIENT_SECRET=xxxxxxx
CODER_EXTERNAL_AUTH_0_AUTH_URL="https://app.vssps.visualstudio.com/oauth2/authorize"
CODER_EXTERNAL_AUTH_0_TOKEN_URL="https://app.vssps.visualstudio.com/oauth2/token"
```

### GitLab self-managed

GitLab self-managed requires the following environment variables:

```env
CODER_EXTERNAL_AUTH_0_ID="primary-gitlab"
CODER_EXTERNAL_AUTH_0_TYPE=gitlab
# This value is the "Application ID"
CODER_EXTERNAL_AUTH_0_CLIENT_ID=xxxxxx
CODER_EXTERNAL_AUTH_0_CLIENT_SECRET=xxxxxxx
CODER_EXTERNAL_AUTH_0_VALIDATE_URL="https://gitlab.company.org/oauth/token/info"
CODER_EXTERNAL_AUTH_0_AUTH_URL="https://gitlab.company.org/oauth/authorize"
CODER_EXTERNAL_AUTH_0_TOKEN_URL="https://gitlab.company.org/oauth/token"
CODER_EXTERNAL_AUTH_0_REGEX=gitlab\.company\.org
```

### Gitea

```env
CODER_EXTERNAL_AUTH_0_ID="gitea"
CODER_EXTERNAL_AUTH_0_TYPE=gitea
CODER_EXTERNAL_AUTH_0_CLIENT_ID=xxxxxxx
CODER_EXTERNAL_AUTH_0_CLIENT_SECRET=xxxxxxx
# If self managed, set the Auth URL to your Gitea instance
CODER_EXTERNAL_AUTH_0_AUTH_URL="https://gitea.com/login/oauth/authorize"
```

### Self-managed git providers

Custom authentication and token URLs should be used for self-managed Git
provider deployments.

```env
CODER_EXTERNAL_AUTH_0_AUTH_URL="https://github.example.com/oauth/authorize"
CODER_EXTERNAL_AUTH_0_TOKEN_URL="https://github.example.com/oauth/token"
CODER_EXTERNAL_AUTH_0_VALIDATE_URL="https://your-domain.com/oauth/token/info"
CODER_EXTERNAL_AUTH_0_REGEX=github\.company\.org
```

> Note: The `REGEX` variable must be set if using a custom git domain.

### Custom scopes

Optionally, you can request custom scopes:

```env
CODER_EXTERNAL_AUTH_0_SCOPES="repo:read repo:write write:gpg_key"
```

### Multiple External Providers (enterprise)

Multiple providers are an Enterprise feature. [Learn more](../enterprise.md).
Below is an example configuration with multiple providers.

```env
# Provider 1) github.com
CODER_EXTERNAL_AUTH_0_ID=primary-github
CODER_EXTERNAL_AUTH_0_TYPE=github
CODER_EXTERNAL_AUTH_0_CLIENT_ID=xxxxxx
CODER_EXTERNAL_AUTH_0_CLIENT_SECRET=xxxxxxx
CODER_EXTERNAL_AUTH_0_REGEX=github.com/orgname

# Provider 2) github.example.com
CODER_EXTERNAL_AUTH_1_ID=secondary-github
CODER_EXTERNAL_AUTH_1_TYPE=github
CODER_EXTERNAL_AUTH_1_CLIENT_ID=xxxxxx
CODER_EXTERNAL_AUTH_1_CLIENT_SECRET=xxxxxxx
CODER_EXTERNAL_AUTH_1_REGEX=github.example.com
CODER_EXTERNAL_AUTH_1_AUTH_URL="https://github.example.com/login/oauth/authorize"
CODER_EXTERNAL_AUTH_1_TOKEN_URL="https://github.example.com/login/oauth/access_token"
CODER_EXTERNAL_AUTH_1_VALIDATE_URL="https://github.example.com/api/v3/user"
```

To support regex matching for paths (e.g. github.com/orgname), you'll need to
add this to the
[Coder agent startup script](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/agent#startup_script):

```shell
git config --global credential.useHttpPath true
```

## Require git authentication in templates

If your template requires git authentication (e.g. running `git clone` in the
[startup_script](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/agent#startup_script)),
you can require users authenticate via git prior to creating a workspace:

![Git authentication in template](../images/admin/git-auth-template.png)

The following example will require users authenticate via GitHub and auto-clone
a repo into the `~/coder` directory.

```hcl
data "coder_external_auth" "github" {
  # Matches the ID of the external auth provider in Coder.
  id = "github"
}

resource "coder_agent" "dev" {
  os   = "linux"
  arch = "amd64"
  dir  = "~/coder"
  env = {
    GITHUB_TOKEN : data.coder_external_auth.github.access_token
  }
  startup_script = <<EOF
if [ ! -d ~/coder ]; then
    git clone https://github.com/coder/coder
fi
EOF
}
```

See the
[Terraform provider documentation](https://registry.terraform.io/providers/coder/coder/latest/docs/data-sources/git_auth)
for all available options.
