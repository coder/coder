# User secrets

User secrets let you store secret values in Coder and make them available in every workspace you own.

## How user secrets work

Each user secret has:

- A name, used to manage the secret with the CLI or REST API.
- A value, which contains the sensitive content.
- An optional description.
- An optional environment variable target, file target, or both.
- An `enabled` setting that records whether Coder should inject the secret through its allowed targets.

An enabled secret must have at least one effective target.
To keep a secret stored without injecting it, set `enabled = false` instead of clearing every target.
Coder rejects a create or update that would leave an enabled secret without an effective target.

Your deployment administrator can turn off Coder-managed file path delivery.
When file path delivery is turned off, environment variable targets continue to work, but stored file paths are blocked.
A file-only secret then has no effective target, even if its stored `enabled` setting is `true`.

Disabled secrets stay visible and editable in the CLI, REST API, and dashboard, but Coder doesn't inject them into workspaces.
Secrets that predate the `enabled` setting and had no target were migrated to `enabled = false`, so they need a target before you can enable them.

User secrets apply to all workspaces that you own.
Secret values are omitted from CLI output and REST API responses after you create or update them.

> [!WARNING]
> Anyone with shell or file access to a workspace can read secrets injected into that workspace.
> Do not share a workspace that has injected secrets with users who should not access those values.
> Turning off Coder-managed file path delivery is not a filesystem security boundary.
> A user who receives a secret through an environment variable can still write the value to a file.

### Storage and encryption

Coder stores user secret values in the database. When
[database encryption](../admin/security/database-encryption.md) is enabled,
Coder encrypts secret values at rest. Otherwise, values are stored in plaintext
in the database.

## How your secrets reach a workspace

The workspace agent receives user secrets in its manifest when it connects to Coder.
The agent fetches another manifest after it reconnects, including after the workspace or agent restarts.
Coder doesn't push a policy or secret change immediately to an agent that remains connected.
Restart the workspace or agent when you need to force a new manifest fetch.

When an administrator turns off file path delivery, each agent stops receiving file targets after its next manifest fetch.
Environment variable targets continue to reach the agent.
Coder omits file-only secrets from the manifest so their plaintext values aren't transmitted without an effective target.

Running shells, apps, startup scripts, and other processes keep any secret values they already received.
Files that Coder wrote before the policy changed also stay on disk.
A reconnect or restart doesn't delete those files.

Running `coder secret disable` prevents delivery after the next manifest fetch, but it doesn't remove values from existing processes or files.
Coder controls its delivery mechanisms, not whether a credential remains valid.
If a credential is exposed or no longer needed, rotate or revoke it in the issuing system and remove stale copies from your workspaces.

### Environment variable secrets

Coder injects environment variable secrets into every new shell, terminal, app, SSH session, and startup script that you start in your workspace.
Existing shells and processes keep the environment they were given when they started.
Turning off file path delivery doesn't affect environment variable delivery.

| If you...                             | ...then in your workspace                                                                                                                                               |
|---------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Create or update an env secret        | The change applies after the next manifest fetch. Until then, existing processes continue to use the values they already received.                                      |
| Rename the env var (`--env NEW_NAME`) | After the next manifest fetch, new shells and processes get `NEW_NAME`. The old name isn't set in new processes.                                                        |
| Clear the env target (`--env ""`)     | The request succeeds if the secret keeps an effective file target or the same request sets `enabled = false`. A stored but blocked file path isn't an effective target. |
| Run `coder secret disable`            | After the next manifest fetch, new shells and processes don't receive the variable. Existing processes keep their previous value.                                       |
| Delete the secret                     | After the next manifest fetch, new shells and processes don't receive the variable. Existing processes keep their previous value.                                       |

To pick up a change in a long-running shell or app started after a restart,
restart that shell or app.

### File secrets

When file path delivery is available, Coder writes file secrets before workspace startup scripts run.
Coder creates missing parent directories.
If the file already exists, Coder overwrites the contents and leaves its existing permissions unchanged.

A deployment administrator can turn off this delivery mechanism.
Coder then rejects new file paths and changes to stored file paths.
You can still clear a stored path with `--file ""`.
Coder preserves legacy paths in the database so you can identify and remove them, and so they can become effective again if an administrator turns file delivery back on.

For a dual-target secret, the environment variable remains effective while its stored file path is blocked.
For a file-only secret, Coder sends neither the target nor its plaintext value to the workspace agent.
The stored `enabled` setting doesn't make a blocked file-only secret effective.

| If you...                                     | ...then in your workspace                                                                                                                                                                        |
|-----------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Create or update a file secret                | When delivery is available, Coder writes or overwrites the file after the next manifest fetch. The server rejects a new path when delivery is turned off.                                        |
| Change the file path (`--file NEW_PATH`)      | When delivery is available, Coder writes the new path after the next manifest fetch. **The previous file stays on disk with its old value.**                                                     |
| Clear the file target (`--file ""`)           | Coder removes the stored target. **A previously written file stays on disk with its last value.** The secret must retain an effective environment target or become disabled in the same request. |
| Run `coder secret disable`                    | Coder stops sending the target after the next manifest fetch. **A previously written file stays on disk with its last value.**                                                                   |
| An administrator turns off file path delivery | Coder blocks the stored path after the next manifest fetch. **A previously written file stays on disk.** Environment delivery continues for a dual-target secret.                                |
| An administrator turns file path delivery on  | The preserved target becomes effective after the next manifest fetch. Coder writes the current secret value and can overwrite a file that changed while delivery was off.                        |
| Delete the secret                             | Coder stops sending the target after the next manifest fetch. **A previously written file stays on disk with its last value.**                                                                   |

> [!IMPORTANT]
> Coder never deletes secret files it has written for you.
> If you remove a secret, change or clear its path, or an administrator turns off file delivery, remove stale files from every affected workspace.
> Run `rm <path>` from the workspace, or use another cleanup mechanism that your organization controls.
> Rebuilding a workspace removes a stale file only when the template recreates the filesystem that contains it.

Coder rejects a second secret that uses a file path you already use.
Two different paths can still resolve to the same absolute path (for example, `~/config` and `/home/coder/config`).
Coder accepts both, but only the last value written remains on disk.
The workspace agent logs a warning for this collision.
Use distinct paths.

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

Only secrets with an environment variable target count against the environment-injected budget.
Coder injects these values into the workspace agent's process environment, which has a limited total size on Windows.
The 24&nbsp;KiB ceiling leaves room for Coder variables and template-defined environment variables.

If several environment targets exhaust the aggregate budget, move appropriate values to file targets only when your deployment permits file path delivery.
If file path delivery is turned off, reduce the values or number of environment-delivered secrets, or use an external secrets manager that the workspace accesses directly.
The per-secret value limit remains 24&nbsp;KiB for every delivery method.

These caps measure stored bytes, which is what Coder writes to the database.
In deployments with
[database encryption](../admin/security/database-encryption.md) enabled,
stored bytes exceed the raw value.

## Manage secrets from the dashboard

You can create, edit, and delete user secrets from the Coder dashboard:

1. Select your avatar.
2. Select **Account**.
3. Select **Secrets**.

The dashboard derives effective delivery from the stored secret and the deployment policy.
When file path delivery is turned off, the form doesn't allow new file targets.
Legacy file paths remain visible as blocked targets so you can remove them.
A legacy dual-target secret continues environment delivery.
A legacy file-only secret is marked as not effectively injected.

The dashboard prevents you from enabling a disabled file-only secret until you add an environment variable target.
You can add an environment target, clear a legacy file path, disable the secret, or delete it.
An unrelated edit doesn't clear a stored path or change the stored `enabled` setting.

The rest of this guide shows equivalent CLI commands.
The server enforces the same policy for the dashboard, CLI, and REST API.

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

If your deployment permits file path delivery, use `--file` to deliver a secret as a file.
File paths must start with `~/` or `/`.
The server rejects this request when a deployment administrator has turned off file path delivery.

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

If your deployment permits file path delivery, you can deliver the same secret through both target types.
If an administrator later turns off file path delivery, the environment target remains effective and Coder preserves the blocked file path.

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

An enabled secret must have at least one target that the deployment permits.
To store a secret without injecting it, pass `--enabled=false`.
You can add an environment target and enable it later with `coder secret enable`.
A stored file path alone isn't effective while file path delivery is turned off.

```sh
echo -n "$API_KEY" | coder secret create api-key --enabled=false
```

## Update a secret

Use `coder secret update` to update a secret value, description, environment variable target, file target, or stored enabled intent.
At least one of `--value`, `--description`, `--env`, `--file`, or `--enabled` must be specified.

When file path delivery is turned off, the server rejects a new path and a change to a stored path.
It permits `--file ""` so you can remove a legacy target.
Resending the exact stored path is allowed, but it doesn't make the target effective.
Unrelated updates preserve the path because the CLI omits fields that you didn't specify.

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
Coder rejects an update that uses a target stored by another secret.
Clearing a target frees it for your other secrets to use.
If another secret takes the target, setting the original target back is rejected until you free it again.

### Enable and disable a secret

Run `coder secret disable` to set the stored intent to disabled without deleting the secret.
Run `coder secret enable` to set the stored intent to enabled.
The server rejects an enable request when the secret has no target that the deployment permits.
While file path delivery is turned off, add an environment target before you enable a file-only secret.

The `enabled` value in CLI and API output is stored intent.
It isn't proof that delivery is effective because a deployment policy can block the only stored target.

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

The list and show commands return secret metadata only.
They never return the secret value.
The `coder secret list` table shows `enabled` as stored intent because deployment policy can block file delivery.

Refer to [How your secrets reach a workspace](#how-your-secrets-reach-a-workspace) for the effects on running workspaces and existing files.

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

Keys that are not valid environment variable names, such as `MY-TOKEN` or the reserved name `PATH`, are imported without an environment variable target.
They are stored with `enabled = false` and aren't injected until you add an effective target.
While file path delivery is turned off, that target must be an environment variable.

To import secrets programmatically, use the [Secrets API](../reference/api/secrets.md#import-user-secrets-from-a-file).

## Migrate blocked file targets

If your administrator turns off file path delivery, review each secret that has a stored path:

1. Confirm that the environment variable works for each dual-target secret.
2. Add an environment target to each file-only secret when the application can consume one.
3. Clear the stored path with `coder secret update <name> --file ""` when you don't need rollback behavior.
4. Set `enabled = false` or delete each file-only secret that can't use an environment variable.
5. Remove files and cached credentials from every workspace that previously received the secret.
6. Rotate or revoke the credential in its issuing system when a stale copy might remain accessible.

If the administrator turns file path delivery back on, any preserved path becomes eligible after the next agent manifest fetch.
The agent writes the current stored value and can overwrite changes made to that file while delivery was turned off.
Coordinate rollback with secret owners before the configuration changes.

In a high-availability deployment, configure the same value on every Coder replica.
Replicas with different values can return different manifests during reconnects.
