# User secrets (Early Access)

User secrets let you store secret values in Coder and make them available in
every workspace you own.

> [!NOTE]
> User secrets are in Early Access and may change. For more information, see
> [feature stages](../install/releases/feature-stages.md#early-access-features).

## How user secrets work

Each user secret has:

- A name, used to manage the secret with the CLI or REST API.
- A value, which contains the sensitive content.
- An optional description.
- An optional environment variable target, file target, or both.

A secret without an environment variable target or file target is stored, but is
not injected into workspaces.

User secrets apply to all workspaces that you own. Coder injects user secrets
when a workspace starts. If you create, update, or delete a secret while a
workspace is running, restart the workspace before relying on that change.

Environment variable secrets are available to startup scripts and workspace
sessions. File secrets are written before startup scripts run.

Secret values are omitted from CLI output and REST API responses after you
create or update them.

> [!WARNING]
> Anyone with shell or file access to a workspace can read secrets injected into
> that workspace. Do not share a workspace that has injected secrets with users
> who should not access those values.

## Create a secret

Use `coder secret create <name>` to create a user secret. For sensitive values,
provide the value through non-interactive stdin with a pipe or redirect. This
keeps the value out of your shell history and process arguments.

### Create an environment variable secret

Use `--env` to inject a secret into your workspaces as an environment variable.
The secret is available under the environment variable name you provide. User
secret environment variables take precedence over template-defined environment
variables with the same name, including variables set with `coder_env`.

```sh
echo -n "$API_KEY" | coder secret create api-key \
  --description "API key for workspace tools" \
  --env API_KEY
```

### Create a file secret

Use `--file` to inject a secret as a file in your workspaces. File paths must
start with `~/` or `/`.

```sh
coder secret create tool-config \
  --description "Tool configuration" \
  --file ~/.config/tool/config.json \
  < ./tool-config.json
```

Coder creates parent directories as needed. If the file already exists, including
a file created by a template or image, Coder updates the contents and preserves
the existing permissions.

### Create a secret with environment variable and file targets

You can inject the same secret as both an environment variable and a file:

```sh
echo -n "$TOKEN" | coder secret create service-token \
  --description "Service token for workspace tools" \
  --env SERVICE_TOKEN \
  --file ~/.config/service/token
```

### Use `--value`

You can also provide a secret value with `--value`:

```sh
coder secret create api-key \
  --value "$API_KEY" \
  --description "API key for workspace tools" \
  --env API_KEY
```

For sensitive values, prefer stdin because `--value` can expose the secret in
shell history or process arguments.

Stdin is read verbatim. If the source file ends with a trailing newline, Coder
stores that newline as part of the secret value. Use `echo -n` when you do not
want to store a trailing newline:

```sh
echo -n "$API_KEY" | coder secret create api-key --env API_KEY
```

### Import secrets in the dashboard

To import one or more secrets from a file, navigate to **User settings** >
**Secrets**, click **Add secret**, then click **Upload**. The dashboard supports
`.env`, `.json`, `.yaml`, and `.yml` files.

For `.env` files, add one secret per line:

```dotenv
GITHUB_TOKEN=example-value
ANTHROPIC_API_KEY=another-example-value
```

For each line, Coder uses the text before the first `=` as both the secret name
and environment variable target. The value is everything after the first `=`.
Blank lines and lines beginning with `#` are ignored.

For JSON and YAML object files, Coder uses each key as both the secret name and
environment variable target:

```json
{
  "GITHUB_TOKEN": "example-value",
  "ANTHROPIC_API_KEY": "another-example-value"
}
```

```yaml
GITHUB_TOKEN: example-value
ANTHROPIC_API_KEY: another-example-value
```

Use a JSON or YAML array when you need to set optional metadata or file targets.
Each item must include `name` and `value`. Items may include `description`,
`env_name`, and `file_path`:

```json
[
  {
    "name": "github",
    "value": "example-value",
    "env_name": "GITHUB_TOKEN",
    "description": "GitHub token",
    "file_path": "~/secrets/github"
  }
]
```

```yaml
- name: github
  value: example-value
  env_name: GITHUB_TOKEN
  description: GitHub token
  file_path: ~/secrets/github
```

> [!NOTE]
> YAML parses unquoted values such as `true`, `false`, and `123` as booleans or
> numbers. Secret imports require string values, so quote values that look like
> booleans or numbers, for example `TOKEN: "123"` or `ENABLED: "true"`.

## Update a secret

Use `coder secret update` to update a secret value, description, environment
variable target, or file target. At least one of `--value`, `--description`,
`--env`, or `--file` must be specified.

```sh
# Update a secret value.
echo -n "$NEW_API_KEY" | coder secret update api-key

# Change the environment variable target.
coder secret update api-key --env NEW_API_KEY

# Clear the file injection target while keeping the secret.
coder secret update api-key --file ""
```

## List and delete secrets

List, show, and delete your secrets with the `coder secret` CLI:

```sh
# List all of your secrets.
coder secret list

# Show a single secret by name.
coder secret list api-key

# Delete a secret you no longer need.
coder secret delete api-key
```

Deleting a secret removes it from Coder and stops Coder from injecting it during
future workspace starts. Deleting a secret does not remove the value from
running processes or delete files that were already written in existing
workspaces.

The list and show commands return secret metadata only. They never return the
secret value.

For full command details, see [`coder secret`](../reference/cli/secret.md) and
the [Secrets API reference](../reference/api/secrets.md).
