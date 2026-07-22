# User secrets (Beta)

User secrets let you store secret values in Coder and make them available in
every workspace you own.

> [!NOTE]
> User secrets are in Beta and may change. For more information, see
> [feature stages](../install/releases/feature-stages.md#beta).

## How user secrets work

Each user secret has:

- A name, used to manage the secret with the CLI or REST API.
- A value, which contains the sensitive content.
- An optional description.
- An optional environment variable target, file target, or both.

A secret without an environment variable target or file target is stored, but is
not injected into workspaces.

User secrets apply to all workspaces that you own.

Secret values are omitted from CLI output and REST API responses after you
create or update them.

> [!WARNING]
> Anyone with shell or file access to a workspace can read secrets injected into
> that workspace. Do not share a workspace that has injected secrets with users
> who should not access those values.

### Storage and encryption

Coder stores user secret values in the database. When
[database encryption](../admin/security/database-encryption.md) is enabled,
Coder encrypts secret values at rest. Otherwise, values are stored in plaintext
in the database.

## How your secrets reach a workspace

Coder applies your secrets when your workspace starts. The same applies any
time the workspace agent reconnects to Coder, for example after the workspace
or the agent restarts. To pick up a change to a secret while a workspace is
running, restart the workspace.

### Environment variable secrets

Coder injects environment variable secrets into every new shell, terminal,
app, SSH session, and startup script that you start in your workspace.
Existing shells and processes keep the environment they were given when they
started.

| If you...                                              | ...then in your workspace                                                                                                                       |
|--------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------|
| Create or update an env secret                         | The change applies after the next workspace start. Until then, your running workspace continues to use the secrets it had when it last started. |
| Rename the env var (`--env NEW_NAME`)                  | After the next workspace start, new shells get `NEW_NAME` and the old name is no longer set.                                                    |
| Clear the env target (`--env ""`) or delete the secret | After the next workspace start, the variable is no longer injected.                                                                             |

To pick up a change in a long-running shell or app started after a restart,
restart that shell or app.

### File secrets

Coder writes file secrets to your workspace filesystem when the workspace
starts, before any startup scripts run. New parent directories are created as
needed. If the file already exists, Coder overwrites the contents and leaves
the existing permissions alone.

| If you...                                                | ...then in your workspace                                                                                                         |
|----------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------|
| Create or update a file secret                           | The file is written or overwritten at the next workspace start.                                                                   |
| Change the file path (`--file NEW_PATH`)                 | At the next workspace start, a file is written at `NEW_PATH`. **The file at the previous path stays on disk with its old value.** |
| Clear the file target (`--file ""`) or delete the secret | **The previously-written file stays on disk with its last value.**                                                                |

> [!IMPORTANT]
> Coder never deletes secret files it has written for you. If you remove a
> secret, change its file path, or clear the file target, the previous file
> stays in your workspace until you delete it. To remove a stale file, open
> a terminal in your workspace and run `rm <path>`. Rebuilding the workspace
> may clear stale files when your template recreates the filesystem.

If you set two file secrets that resolve to the same absolute path (for
example `~/config` and `/home/coder/config`), only one of them ends up on
disk; the workspace agent logs a warning to help spot this. Use
distinct paths to avoid the collision.

## Limits

User secrets are subject to the following limits. Coder enforces these when you
create or update a secret and rejects the request with an explanatory 400 when
you exceed one. Delete or shrink an existing secret to make room.

| Cap                                      | Value     |
|------------------------------------------|-----------|
| Total secrets per user                   | 50        |
| Combined stored value bytes per user     | 200 KiB   |
| Combined stored env-injected value bytes | 24 KiB    |
| Per-secret value bytes                   | 24 KiB    |
| Env var name length                      | 256 bytes |

Only secrets created with `--env` count against the env-injected budget. Coder
injects these into the workspace agent's process environment, which on Windows
has a ~32 KiB total budget. The 24 KiB ceiling leaves room for Coder's own
variables (`CODER_*`, `PATH`, `HOME`, ...) plus any template-defined env. To
inject a value larger than this budget, use `--file` instead; file secrets do
not count against the env budget.

The per-secret cap matches the env aggregate cap because a value larger than
the env aggregate could never be injected successfully as an environment
variable.

These caps measure stored bytes, which is what Coder writes to the database.
In deployments with
[database encryption](../admin/security/database-encryption.md) enabled,
stored bytes exceed the raw value.

## Manage secrets from the dashboard

You can create, edit, and delete user secrets from the Coder dashboard:

1. Click your avatar in the top right.
1. Select **Account**.
1. Select **Secrets**.

From this page you can add a new secret, update an existing secret's value,
description, or environment variable and file targets, and delete secrets you
no longer need.

The rest of this guide shows the equivalent CLI commands. The same behaviors,
limits, and injection rules apply whether you manage secrets from the
dashboard or the CLI.

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

On Windows workspaces, prefer `~/...` paths. They resolve to your Windows
user profile directory. Paths starting with `/` are accepted but resolve
to the root of the workspace's current drive, which is template dependent.

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

The list and show commands return secret metadata only. They never return the
secret value.

See [How your secrets reach a workspace](#how-your-secrets-reach-a-workspace)
for what happens to running workspaces when you delete a secret.

For full command details, see [`coder secret`](../reference/cli/secret.md) and
the [Secrets API reference](../reference/api/secrets.md).

## Bulk import

Import many secrets at once by uploading a file to the batch API endpoint. This
is useful when you want to move an existing `.env`, JSON, or YAML file of values
into Coder in a single request.

The endpoint is `POST /api/v2/users/{user}/secrets/batch`. The request body is
JSON with two fields:

- `format`: one of `env`, `json`, or `yaml`.
- `content`: the raw file contents, as a string.

The supported formats are:

- `env`: dotenv-style `KEY=VALUE` lines.
- `json`: a flat JSON object of string values, for example
  `{"API_KEY":"...","DB_URL":"..."}`.
- `yaml`: a flat YAML mapping of string values. Multi-document YAML is
  supported.

The following example imports two secrets from an inline env-format file:

```sh
curl -X POST http://coder-server:8080/api/v2/users/me/secrets/batch \
  -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"format":"env","content":"API_KEY=abc123\nDB_URL=postgres://localhost/db\n"}'
```

The same operation is available in the Go SDK as
`codersdk.Client.ImportUserSecrets`. For the full request and response schema,
refer to the "Import user secrets from a file" operation in the
[Secrets API reference](../reference/api/secrets.md).

### How keys become secrets

Each key in the file becomes a secret whose name is the key. When the key is a
valid environment variable name, Coder also sets it as the secret's environment
variable target so the value is injected into your workspaces, exactly like a
secret created individually. Refer to
[How your secrets reach a workspace](#how-your-secrets-reach-a-workspace) for
the injection details.

Keys that are not valid environment variable names (for example `MY-TOKEN`) or
reserved names (for example `PATH`) are still imported, but with an empty
environment variable target. Like any secret without an environment variable or
file target, these are stored but not injected.

### All-or-nothing

Imports are atomic. If any entry fails validation, reuses a name that is already
in use, or would exceed a per-user limit, Coder rolls back the whole batch and
creates nothing. Fix the reported entry and retry the full file.

Secret values are never returned in the response and are omitted from audit
logs. On success, the response is an array of secret metadata (`id`, `name`,
`description`, `env_name`, `file_path`, `created_at`, and `updated_at`), the
same shape returned when you create a single secret.

### Import limits

A single file may contain at most 50 secrets, and the file itself must be no
larger than 1 MiB. Requests whose raw body exceeds the endpoint cap return
HTTP 413. Imported secrets also count against the same per-user budgets
described in [Limits](#limits), so a batch that would push you over those
limits is rejected and nothing is created.
