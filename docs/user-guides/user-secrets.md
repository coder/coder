---
title: User secrets
---

User secrets let you store secret values in Coder and make them available in
every workspace you own.

## How user secrets work

Each user secret has:

- A name, used to manage the secret with the CLI or REST API.
- A value, which contains the sensitive content.
- An optional description.
- An optional environment variable target, file target, or both.
- An enabled setting that records whether the secret should be injected.

An enabled secret needs at least one effective target.
Your deployment administrator can turn off file path delivery, which blocks stored file paths but preserves environment variable delivery and the stored path.
A file-only secret then has no effective target, even when its enabled setting is `true`.

Disabled secrets stay visible and editable in the CLI, REST API, and dashboard,
but are not injected into workspaces. Secrets that predate the enabled flag and
had no target were migrated to disabled, so they show as disabled and need a
target before you can enable them.

User secrets apply to all workspaces that you own.

Secret values are omitted from CLI output and REST API responses after you
create or update them.

> [!WARNING]
> Anyone with shell or file access to a workspace can read injected secrets.
> Turning off Coder-managed file path delivery is not a filesystem boundary because users can write environment variables to disk.

### Storage and encryption

Coder stores user secret values in the database. When
[database encryption](../admin/security/database-encryption.md) is enabled,
Coder encrypts secret values at rest. Otherwise, values are stored in plaintext
in the database.

## How your secrets reach a workspace

Coder applies secrets when the workspace agent fetches its manifest during connection or reconnection.
Restart the workspace or agent to force a fetch; Coder does not immediately refresh a connected agent.
Existing processes keep environment values they already received, and existing files remain on disk.

Coder controls delivery, not credential validity.
Remove stale copies and rotate or revoke credentials in the issuing system.

### Environment variable secrets

Coder injects environment variable secrets into every new shell, terminal,
app, SSH session, and startup script that you start in your workspace.
Existing shells and processes keep the environment they were given when they
started.

| If you...                                   | ...then in your workspace                                                                                                                                                                               |
|---------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Create or update an env secret              | The change applies after the next workspace start. Until then, your running workspace continues to use the secrets it had when it last started.                                                         |
| Rename the env var (`--env NEW_NAME`)       | After the next workspace start, new shells get `NEW_NAME` and the old name is no longer set.                                                                                                            |
| Clear the env target (`--env ""`)           | Only succeeds if the secret keeps its file target or is disabled in the same request; otherwise the request is rejected with a 400. After the next workspace start, the variable is no longer injected. |
| Disable the secret (`coder secret disable`) | After the next workspace start, the variable is no longer injected. Running sessions keep the value until the agent manifest is refetched (workspace restart).                                          |
| Delete the secret                           | After the next workspace start, the variable is no longer injected.                                                                                                                                     |

To pick up a change in a long-running shell or app started after a restart,
restart that shell or app.

### File secrets

When file path delivery is available, Coder writes file secrets before startup scripts run.
If an administrator turns it off, Coder rejects new or changed paths, preserves existing paths, continues environment delivery for dual-target secrets, and omits file-only secret values from agent manifests.
Use `coder secret update <name> --file "" --enabled=false` to remove the only preserved target, or omit `--enabled=false` when an environment target remains.

If delivery is turned on again, a preserved path becomes effective after the next manifest fetch and Coder may overwrite a file changed while delivery was off.

| If you...                                   | ...then in your workspace                                                                                                                                                                             |
|---------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Create or update a file secret              | The file is written or overwritten at the next workspace start.                                                                                                                                       |
| Change the file path (`--file NEW_PATH`)    | At the next workspace start, a file is written at `NEW_PATH`. **The file at the previous path stays on disk with its old value.**                                                                     |
| Clear the file target (`--file ""`)         | Only succeeds if the secret keeps its env target or is disabled in the same request; otherwise the request is rejected with a 400. **The previously-written file stays on disk with its last value.** |
| Disable the secret (`coder secret disable`) | The file is no longer written at the next workspace start. **The previously-written file stays on disk with its last value.**                                                                         |
| Delete the secret                           | **The previously-written file stays on disk with its last value.**                                                                                                                                    |

> [!IMPORTANT]
> Coder never deletes secret files.
> Remove stale files from affected workspaces with `rm <path>` or an organization-managed cleanup process.

Coder rejects a second secret that uses a file path you already use. Two
different paths can still resolve to the same absolute path (for example
`~/config` and `/home/coder/config`). Coder accepts both, but only one of them
ends up on disk; the workspace agent logs a warning to help spot this. Use
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

Only environment-targeted secrets count against the environment budget, which protects the Windows process environment limit.
When file delivery is available, clear `env_name` and use `file_path` for suitable large values.
Otherwise, reduce the environment secrets or use an external secrets manager.

These caps measure stored bytes, which is what Coder writes to the database.
In deployments with
[database encryption](../admin/security/database-encryption.md) enabled,
stored bytes exceed the raw value.

## Manage secrets from the dashboard

You can create, edit, and delete user secrets from the Coder dashboard:

1. Select your avatar in the top right.
1. Select **Account**.
1. Select **Secrets**.

When file delivery is off, the dashboard hides new file targets, marks preserved paths as blocked, shows file-only secrets as not injected, and prevents enabling them until you add an environment target.
You can remove a path, add an environment target, disable the secret, or delete it.

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

If your deployment permits file delivery, use `--file` with a path starting with `~/` or `/`.

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

You can use both targets when file delivery is available.
If it is later turned off, the environment target remains effective and the path stays preserved.

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

### Import multiple secrets from a file

Use `coder secret import <file>` to create a secret for every key in a dotenv,
JSON, or YAML file. The format is inferred from the file extension. Pass `-`
to read from non-interactive stdin, which requires `--input-format`:

```sh
coder secret import ./secrets.env

coder secret import - --input-format yaml < ./secrets.yaml
```

The import is all or nothing and never overwrites existing secrets. Keys that
are valid environment variable names are injected under the same name; other
keys are imported without an environment variable target. For details, see
[`coder secret import`](../reference/cli/secret_import.md).

### Create a disabled secret

Pass `--enabled=false` to store a secret without injecting it.
While file delivery is off, an enabled secret needs an environment target.

```sh
echo -n "$API_KEY" | coder secret create api-key --enabled=false
```

## Update a secret

Use `coder secret update` to change fields.
When file delivery is off, the server rejects new or changed paths but permits `--file ""` cleanup and leaves paths untouched during unrelated edits.

```sh
# Update a secret value.
echo -n "$NEW_API_KEY" | coder secret update api-key

# Change the environment variable target.
coder secret update api-key --env NEW_API_KEY

# Clear the file injection target while keeping the secret. This only
# succeeds because api-key still has an environment variable target; a
# request that clears the last target of an enabled secret is rejected.
coder secret update api-key --file ""
```

Environment variable names and file paths are unique among your own secrets.
Coder rejects an update that uses an environment variable name or file path that another of your secrets already uses.

Clearing a target frees it for your other secrets to use.
If another secret takes it, setting the original target back is rejected until you free it again.

### Enable and disable a secret

Disable a secret to stop injecting it without deleting it, then enable it again
to resume. Enabling a secret that has no target is rejected; add a target
first.

```sh
# Stop injecting a secret without deleting it.
coder secret disable api-key

# Resume injection.
coder secret enable api-key
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

The list and show commands omit secret values.
The `enabled` column is stored intent, not proof of effective delivery.

See [How your secrets reach a workspace](#how-your-secrets-reach-a-workspace)
for what happens to running workspaces when you delete a secret.

## Import secrets from a file

If you keep secrets in a dotenv file, a flat JSON object, or a flat YAML
mapping, you can import the whole file instead of creating each secret
individually:

1. Go to the [**Secrets** page](#manage-secrets-from-the-dashboard) and select
   **Add secret**.
1. Drop or select a `.env`, `.json`, `.yaml`, or `.yml` file in the upload
   area. Coder imports the file as soon as you choose it.

Every key in the file becomes a secret. For example, this dotenv file creates
two secrets, `API_KEY` and `DATABASE_URL`, each injected as an environment
variable of the same name:

```sh
API_KEY=abc123
DATABASE_URL=postgres://user:pass@db.internal/app
```

In JSON and YAML files, every value must be a string. Quote numeric and
boolean values, for example `"PORT": "8080"`.

The import is all or nothing. If any entry fails validation, conflicts with
an existing secret, or exceeds a [limit](#limits), Coder cancels the import
and creates no secrets. The file must also be 1 MiB or smaller and contain no
more than 50 keys.

Keys that are not valid environment variable names, such as `MY-TOKEN` or the
reserved name `PATH`, are imported without an environment variable target.
They are not injected into workspaces until you add a valid environment
variable or file target.

To import secrets programmatically, use the
[Secrets API](../reference/api/secrets.md#import-user-secrets-from-a-file).

## Migrate blocked file targets

1. Confirm environment delivery for dual-target secrets.
2. Add environment targets to file-only secrets when possible.
3. Clear paths that are not needed for rollback.
4. Disable or delete file-only secrets that cannot use environment variables.
5. Remove stale files and rotate or revoke credentials when needed.

Coordinate rollback with secret owners because preserved paths can overwrite workspace files after delivery is turned on again.

For full command details, refer to [`coder secret`](../reference/cli/secret.md) and the [Secrets API](../reference/api/secrets.md).
