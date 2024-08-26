# Configure Control Plane Access

Coder server's primary configuration is done via environment variables. For a
full list of the options, run `coder server --help` or see our
[CLI documentation](../reference/cli/server.md).

## Access URL

`CODER_ACCESS_URL` is required if you are not using the tunnel. Set this to the
external URL that users and workspaces use to connect to Coder (e.g.
<https://coder.example.com>). This should not be localhost.

> Access URL should be an external IP address or domain with DNS records
> pointing to Coder.

### Tunnel

If an access URL is not specified, Coder will create a publicly accessible URL
to reverse proxy your deployment for simple setup.

## Address

You can change which port(s) Coder listens on.

```shell
# Listen on port 80
export CODER_HTTP_ADDRESS=0.0.0.0:80

# Enable TLS and listen on port 443)
export CODER_TLS_ENABLE=true
export CODER_TLS_ADDRESS=0.0.0.0:443

## Redirect from HTTP to HTTPS
export CODER_REDIRECT_TO_ACCESS_URL=true

# Start the Coder server
coder server
```

## Wildcard access URL

`CODER_WILDCARD_ACCESS_URL` is necessary for
[port forwarding](../networking/port-forwarding.md#dashboard) via the dashboard
or running [coder_apps](../templates/index.md#coder-apps) on an absolute path.
Set this to a wildcard subdomain that resolves to Coder (e.g.
`*.coder.example.com`).

If you are providing TLS certificates directly to the Coder server, either

1. Use a single certificate and key for both the root and wildcard domains.
2. Configure multiple certificates and keys via
   [`coder.tls.secretNames`](https://github.com/coder/coder/blob/main/helm/coder/values.yaml)
   in the Helm Chart, or
   [`--tls-cert-file`](../reference/cli/server.md#--tls-cert-file) and
   [`--tls-key-file`](../reference/cli/server.md#--tls-key-file) command line
   options (these both take a comma separated list of files; list certificates
   and their respective keys in the same order).

## TLS & Reverse Proxy

The Coder server can directly use TLS certificates with `CODER_TLS_ENABLE` and
accompanying configuration flags. However, Coder can also run behind a
reverse-proxy to terminate TLS certificates from LetsEncrypt, for example.

- [Apache](https://github.com/coder/coder/tree/main/examples/web-server/apache)
- [Caddy](https://github.com/coder/coder/tree/main/examples/web-server/caddy)
- [NGINX](https://github.com/coder/coder/tree/main/examples/web-server/nginx)

### Kubernetes TLS configuration

Below are the steps to configure Coder to terminate TLS when running on
Kubernetes. You must have the certificate `.key` and `.crt` files in your
working directory prior to step 1.

1. Create the TLS secret in your Kubernetes cluster

```shell
kubectl create secret tls coder-tls -n <coder-namespace> --key="tls.key" --cert="tls.crt"
```

> You can use a single certificate for the both the access URL and wildcard
> access URL. The certificate CN must match the wildcard domain, such as
> `*.example.coder.com`.

1. Reference the TLS secret in your Coder Helm chart values

```yaml
coder:
  tls:
    secretName:
      - coder-tls

  # Alternatively, if you use an Ingress controller to terminate TLS,
  # set the following values:
  ingress:
    enable: true
    secretName: coder-tls
    wildcardSecretName: coder-tls
```

## PostgreSQL Database

Coder uses a PostgreSQL database to store users, workspace metadata, and other
deployment information. Use `CODER_PG_CONNECTION_URL` to set the database that
Coder connects to. If unset, PostgreSQL binaries will be downloaded from Maven
(<https://repo1.maven.org/maven2>) and store all data in the config root.

> Postgres 13 is the minimum supported version.

If you are using the built-in PostgreSQL deployment and need to use `psql` (aka
the PostgreSQL interactive terminal), output the connection URL with the
following command:

```console
coder server postgres-builtin-url
psql "postgres://coder@localhost:49627/coder?sslmode=disable&password=feU...yI1"
```

### Migrating from the built-in database to an external database

To migrate from the built-in database to an external database, follow these
steps:

1. Stop your Coder deployment.
2. Run `coder server postgres-builtin-serve` in a background terminal.
3. Run `coder server postgres-builtin-url` and copy its output command.
4. Run `pg_dump <built-in-connection-string> > coder.sql` to dump the internal
   database to a file.
5. Restore that content to an external database with
   `psql <external-connection-string> < coder.sql`.
6. Start your Coder deployment with
   `CODER_PG_CONNECTION_URL=<external-connection-string>`.

## System packages

If you've installed Coder via a [system package](../install/cli.md), you can
configure the server by setting the following variables in
`/etc/coder.d/coder.env`:

```env
# String. Specifies the external URL (HTTP/S) to access Coder.
CODER_ACCESS_URL=https://coder.example.com

# String. Address to serve the API and dashboard.
CODER_HTTP_ADDRESS=0.0.0.0:3000

# String. The URL of a PostgreSQL database to connect to. If empty, PostgreSQL binaries
# will be downloaded from Maven (https://repo1.maven.org/maven2) and store all
# data in the config root. Access the built-in database with "coder server postgres-builtin-url".
CODER_PG_CONNECTION_URL=

# Boolean. Specifies if TLS will be enabled.
CODER_TLS_ENABLE=

# If CODER_TLS_ENABLE=true, also set:
CODER_TLS_ADDRESS=0.0.0.0:3443

# String. Specifies the path to the certificate for TLS. It requires a PEM-encoded file.
# To configure the listener to use a CA certificate, concatenate the primary
# certificate and the CA certificate together. The primary certificate should
# appear first in the combined file.
CODER_TLS_CERT_FILE=

# String. Specifies the path to the private key for the certificate. It requires a
# PEM-encoded file.
CODER_TLS_KEY_FILE=
```

To run Coder as a system service on the host:

```shell
# Use systemd to start Coder now and on reboot
sudo systemctl enable --now coder

# View the logs to ensure a successful start
journalctl -u coder.service -b
```

To restart Coder after applying system changes:

```shell
sudo systemctl restart coder
```

## Configuring Coder behind a proxy

To configure Coder behind a corporate proxy, set the environment variables
`HTTP_PROXY` and `HTTPS_PROXY`. Be sure to restart the server. Lowercase values
(e.g. `http_proxy`) are also respected in this case.

<!-- TODO. Should we Split all configurations in separate guides? -->

## External Authentication

To add an external authentication provider, you'll need to create an OAuth
application. The following providers are supported:

- [GitHub](#github)
- [GitLab](https://docs.gitlab.com/ee/integration/oauth_provider.html)
- [BitBucket](https://support.atlassian.com/bitbucket-cloud/docs/use-oauth-on-bitbucket-cloud/)
- [Azure DevOps](https://learn.microsoft.com/en-us/azure/devops/integrate/get-started/authentication/oauth?view=azure-devops)
- [Azure DevOps (via Entra ID)](https://learn.microsoft.com/en-us/entra/architecture/auth-oauth2)

The next step is to configure the Coder server to use the OAuth application by
setting the following environment variables:

```env
CODER_EXTERNAL_AUTH_0_ID="<USER_DEFINED_ID>"
CODER_EXTERNAL_AUTH_0_TYPE=<github|gitlab|azure-devops|bitbucket-cloud|bitbucket-server|etc>
CODER_EXTERNAL_AUTH_0_CLIENT_ID=xxxxxx
CODER_EXTERNAL_AUTH_0_CLIENT_SECRET=xxxxxxx

# Optionally, configure a custom display name and icon
CODER_EXTERNAL_AUTH_0_DISPLAY_NAME="Google Calendar"
CODER_EXTERNAL_AUTH_0_DISPLAY_ICON="https://mycustomicon.com/google.svg"
```

The `CODER_EXTERNAL_AUTH_0_ID` environment variable is used for internal
reference. Therefore, it can be set arbitrarily (e.g., `primary-github` for your
GitHub provider).

### GitHub

> If you don't require fine-grained access control, it's easier to configure a
> GitHub OAuth app!

1. [Create a GitHub App](https://docs.github.com/en/apps/creating-github-apps/registering-a-github-app/registering-a-github-app)

   - Set the callback URL to
     `https://coder.example.com/external-auth/USER_DEFINED_ID/callback`.
   - Deactivate Webhooks.
   - Enable fine-grained access to specific repositories or a subset of
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
   | Members       | Rad-only     | Grabts access to organization members and teams.       |

3. Install the App for your organization. You may select a subset of
   repositories to grant access to.

   ![Install GitHub App](../images/admin/github-app-install.png)

```env
CODER_EXTERNAL_AUTH_0_ID="USER_DEFINED_ID"
CODER_EXTERNAL_AUTH_0_TYPE=github
CODER_EXTERNAL_AUTH_0_CLIENT_ID=xxxxxx
CODER_EXTERNAL_AUTH_0_CLIENT_SECRET=xxxxxxx
```

### GitHub Enterprise

GitHub Enterprise requires the following environment variables:

```env
CODER_EXTERNAL_AUTH_0_ID="primary-github"
CODER_EXTERNAL_AUTH_0_TYPE=github
CODER_EXTERNAL_AUTH_0_CLIENT_ID=xxxxxx
CODER_EXTERNAL_AUTH_0_CLIENT_SECRET=xxxxxxx
CODER_EXTERNAL_AUTH_0_VALIDATE_URL="https://github.example.com/api/v3/user"
CODER_EXTERNAL_AUTH_0_AUTH_URL="https://github.example.com/login/oauth/authorize"
CODER_EXTERNAL_AUTH_0_TOKEN_URL="https://github.example.com/login/oauth/access_token"
```

### Bitbucket Server

Bitbucket Server requires the following environment variables:

```env
CODER_EXTERNAL_AUTH_0_ID="primary-bitbucket-server"
CODER_EXTERNAL_AUTH_0_TYPE=bitbucket-server
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

### Azure DevOps (via Entra ID)

Azure DevOps (via Entra ID) requires the following environment variables:

```env
CODER_EXTERNAL_AUTH_0_ID="primary-azure-devops"
CODER_EXTERNAL_AUTH_0_TYPE=azure-devops-entra
CODER_EXTERNAL_AUTH_0_CLIENT_ID=xxxxxx
CODER_EXTERNAL_AUTH_0_CLIENT_SECRET=xxxxxxx
CODER_EXTERNAL_AUTH_0_AUTH_URL="https://login.microsoftonline.com/<TENANT ID>/oauth2/authorize"
```

> Note: Your app registration in Entra ID requires the `vso.code_write` scope

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

The Redirect URI for Gitea should be
https://coder.company.org/external-auth/gitea/callback

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

### JFrog Artifactory

See [this](../admin/integrations/jfrog-artifactory.md) guide on instructions on
how to set up for JFrog Artifactory.

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

### Kubernetes environment variables

If you deployed Coder with Kubernetes you can set the environment variables in
your `values.yaml` file:

```yaml
coder:
  env:
    # […]
    - name: CODER_EXTERNAL_AUTH_0_ID
      value: USER_DEFINED_ID

    - name: CODER_EXTERNAL_AUTH_0_TYPE
      value: github

    - name: CODER_EXTERNAL_AUTH_0_CLIENT_ID
      valueFrom:
        secretKeyRef:
          name: github-primary-basic-auth
          key: client-id

    - name: CODER_EXTERNAL_AUTH_0_CLIENT_SECRET
      valueFrom:
        secretKeyRef:
          name: github-primary-basic-auth
          key: client-secret
```

You can set the secrets by creating a `github-primary-basic-auth.yaml` file and
applying it.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: github-primary-basic-auth
type: Opaque
stringData:
  client-secret: xxxxxxxxx
  client-id: xxxxxxxxx
```

Make sure to restart the affected pods for the change to take effect.

## Up Next

- [Learn how to upgrade Coder](./upgrade.md).
