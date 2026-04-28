# User secrets (Early Access)

User secrets let you store personal credentials in Coder and make them
available in every workspace you own. Use them for values that belong to you,
such as API keys, cloud credentials, or tool configuration that should not live
in template code.

> [!NOTE]
> User secrets are in Early Access and may change. For more information, see
> [feature stages](../install/releases/feature-stages.md#early-access-features).

## How user secrets work

Coder workspaces are created from templates. Templates define the shared
infrastructure, tools, and workspace configuration for a team or organization.
User secrets sit outside templates: you create a secret once, choose how Coder
should expose it, and Coder injects it into your workspaces.

Each user secret has:

- A name, used to manage the secret with the CLI or API.
- A value, which contains the sensitive content.
- An optional description.
- An optional environment variable target, file target, or both.

A secret without an environment variable target or file target is stored, but is
not injected into workspaces.

User secrets apply to all workspaces that you own. Environment variable secrets
are available to startup scripts and workspace sessions. File secrets are
written before startup scripts run.

Secret values are omitted from CLI and API output after you create or update
them.

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
variables with the same name.

```sh
printf %s "$API_KEY" | coder secret create api-key \
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

Coder creates parent directories as needed. If the file already exists, Coder
updates the contents and preserves the existing permissions.

### Use both injection targets

You can expose the same secret as both an environment variable and a file:

```sh
printf %s "$TOKEN" | coder secret create service-token \
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
stores that newline as part of the secret value. Use `printf %s` when you do not
want to store a trailing newline.

## Update a secret

Use `coder secret update` to update a secret value, description, environment
variable target, or file target. At least one of `--value`, `--description`,
`--env`, or `--file` must be specified.

```sh
# Update a secret value.
printf %s "$NEW_API_KEY" | coder secret update api-key

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

The list and show commands return secret metadata only. They never return the
secret value.

For full command details, see [`coder secret`](../reference/cli/secret.md) and
the [Secrets API reference](../reference/api/secrets.md).
