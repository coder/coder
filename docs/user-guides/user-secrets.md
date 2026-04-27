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

Use `--env` when a tool expects a credential in an environment variable. The
secret is available in your workspaces under the environment variable name you
provide.

```sh
coder secret create api-key \
  --value "$API_KEY" \
  --description "API key for workspace tools" \
  --env API_KEY
```

## File secrets

Use `--file` when a tool expects a credential or config file on disk. File paths
must start with `~/` or `/`.

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

You can update a secret later with `coder secret update`, including rotating the
value or clearing an injection target by passing an empty string. Use
`coder secret delete` to remove a secret entirely. The secret value itself is
never returned by the API or CLI list output. For full command details, see
[`coder secret`](../reference/cli/secret.md) and the
[Secrets API reference](../reference/api/secrets.md).
