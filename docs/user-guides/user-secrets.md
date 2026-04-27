# User secrets (beta)

User secrets let you store personal credentials in Coder and automatically
inject them into your workspaces without adding those values to template code.
They are a good fit for per-user credentials such as API keys, cloud
credentials, or other values that should follow you across workspaces.

> [!NOTE]
> User secrets are in beta and may change. For more information, see
> [feature stages](../install/releases/feature-stages.md#beta).

Use the CLI to create and manage user secrets. You can inject each secret as an
environment variable, a file, or both.

## Environment variable secrets

Use `--env` to inject a secret into your workspaces as an environment
variable. The secret is available under the environment variable name you
provide.

```sh
coder secret create api-key \
  --value "$API_KEY" \
  --description "API key for workspace tools" \
  --env API_KEY
```

## File secrets

Use `--file` to inject a secret as a file in your workspaces. File paths must
start with `~/` or `/`.

```sh
coder secret create tool-config \
  --description "Tool configuration" \
  --file ~/.config/tool/config.json \
  < ./tool-config.json
```

You can also expose the same secret through both injection targets:

```sh
coder secret create service-token \
  --value "$TOKEN" \
  --description "Service token for workspace tools" \
  --env SERVICE_TOKEN \
  --file ~/.config/service/token
```

## Secret values

Provide a secret value with `--value`, or non-interactive stdin (pipe or
redirect). The examples above use `--value` for readability. For sensitive
values, prefer stdin so the value does not appear in shell history or process
arguments:

```sh
coder secret create api-key \
  --description "API key for workspace tools" \
  --env API_KEY \
  < ./api-key.txt
```

Stdin is read verbatim. If the source file ends with a trailing newline, Coder
stores that newline as part of the secret value.

## Update secrets

Use `coder secret update` to rotate a secret value or change where it is
injected. At least one of `--value`, `--description`, `--env`, or `--file` must
be specified.

```sh
# Rotate a secret value.
coder secret update api-key --value "$NEW_API_KEY"

# Clear the file injection target while keeping the secret.
coder secret update api-key --file ""
```

## Manage secrets

List, show, and delete your secrets with the `coder secret` CLI:

```sh
# List all of your secrets.
coder secret list

# Show a single secret by name.
coder secret list api-key

# Delete a secret you no longer need.
coder secret delete api-key
```

The secret value itself is never returned by the API or CLI list output. For
full command details, see [`coder secret`](../reference/cli/secret.md) and the
[Secrets API reference](../reference/api/secrets.md).
