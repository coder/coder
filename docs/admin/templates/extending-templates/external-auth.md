# External Authentication

Coder integrates with any OpenID Connect provider to automate away the need for
developers to authenticate with external services within their workspace. This
can be used to authenticate with git providers, private registries, or any other
service that requires authentication.

## External Auth Providers

External auth providers are configured using environment variables in the Coder
Control Plane. See

## Git Providers

When developers use `git` inside their workspace, they are prompted to
authenticate. After that, Coder will store and refresh tokens for future
operations.

<video autoplay playsinline loop>
  <source src="https://github.com/coder/coder/blob/main/site/static/external-auth.mp4?raw=true" type="video/mp4">
Your browser does not support the video tag.
</video>

### Require git authentication in templates

If your template requires git authentication (e.g. running `git clone` in the
[startup_script](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/agent#startup_script)),
you can require users authenticate via git prior to creating a workspace:

![Git authentication in template](../../../images/admin/git-auth-template.png)

### Native git authentication will auto-refresh tokens

<blockquote class="info">
  <p>
  This is the preferred authentication method.
  </p>
</blockquote>

By default, the coder agent will configure native `git` authentication via the
`GIT_ASKPASS` environment variable. Meaning, with no additional configuration,
external authentication will work with native `git` commands.

To check the auth token being used **from inside a running workspace**, run:

```shell
# If the exit code is non-zero, then the user is not authenticated with the
# external provider.
coder external-auth access-token <external-auth-id>
```

Note: Some IDE's override the `GIT_ASKPASS` environment variable and need to be
configured.

#### VSCode

Use the
[Coder](https://marketplace.visualstudio.com/items?itemName=coder.coder-remote)
extension to automatically configure these settings for you!

Otherwise, you can manually configure the following settings:

- Set `git.terminalAuthentication` to `false`
- Set `git.useIntegratedAskPass` to `false`

### Hard coded tokens do not auto-refresh

If the token is required to be inserted into the workspace, for example
[GitHub cli](https://cli.github.com/), the auth token can be inserted from the
template. This token will not auto-refresh. The following example will
authenticate via GitHub and auto-clone a repo into the `~/coder` directory.

```tf
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
[Terraform provider documentation](https://registry.terraform.io/providers/coder/coder/latest/docs/data-sources/external_auth)
for all available options.
