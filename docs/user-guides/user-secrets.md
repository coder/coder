# User secrets

User secrets let you store secret values in Coder and make them available in
every workspace you own.

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

Only secrets that have an environment variable target count against the env-injected budget.
Coder injects these into the workspace agent's process environment, which on Windows has a ~32 KiB total budget.
The 24 KiB ceiling leaves room for Coder's own variables (`CODER_*`, `PATH`, `HOME`, ...) plus any template-defined env.
Imported secrets count too, because an import sets an environment variable target for every key that is a valid environment variable name.
To inject a value larger than this budget, give the secret a file target instead, because file secrets don't count against the env budget.
An import never sets a file target, so move an imported secret to a file by editing it, or create the secret with `coder secret create --file`.

The per-secret cap matches the env aggregate cap because a value larger than
the env aggregate could never be injected successfully as an environment
variable.

These caps measure stored bytes, which is what Coder writes to the database.
In deployments with
[database encryption](../admin/security/database-encryption.md) enabled,
stored bytes exceed the raw value.

## Manage secrets from the dashboard

You can create, edit, and delete user secrets from the Coder dashboard:

1. Select your avatar in the top right.
1. Select **Account**.
1. Select **Secrets**.

From this page you can add a new secret, import secrets from a file, update an existing secret's value, description, or environment variable and file targets, and delete secrets you no longer need.

## Import secrets from a file

If you keep secrets in a dotenv file, a flat JSON object, or a flat YAML mapping, you can import the whole file instead of creating each secret one at a time.
You can import from the dashboard or [with the API](#import-with-the-api).

To import a file from the dashboard:

1. Go to the [**Secrets** page](#manage-secrets-from-the-dashboard) and select **Add secret**.
1. Drop or select your file in the **Import secrets from a file** area of the dialog.

Coder imports the file as soon as you choose it, so there's no separate confirm step.
The **Save** button applies only to the single-secret fields in the same dialog.
On success, the dialog closes and a toast reports how many secrets were imported.
If the import fails, the dialog stays open and shows the error.
To choose a different file, select **Remove file** first.

Every key in the file becomes a secret.
This dotenv file creates 2 secrets, `API_KEY` and `DATABASE_URL`, each injected into your workspaces as an environment variable of the same name:

```dotenv
API_KEY=abc123
DATABASE_URL=postgres://user:pass@db.internal/app
```

### Choose the file format

Coder picks the parser from the filename, not from the file contents.
The filename must end in `.env`, `.json`, `.yaml`, or `.yml`, matched without regard to case.

| Format | Filename ends in | Multiline values                    |
|--------|------------------|-------------------------------------|
| `env`  | `.env`           | Not supported                       |
| `json` | `.json`          | Supported with `\n` inside a string |
| `yaml` | `.yaml`, `.yml`  | Supported with a block scalar       |

A filename that ends in anything else isn't recognized, including `.env.local`, `.env.production`, and a file with no extension at all.
Rename the file to something like `local.env` first.
A file named exactly `.env` works, but your operating system's file picker may hide dotfiles, so you may need to reveal hidden files or drag the file into the dialog.

With the API you pass the format explicitly as `env`, `json`, or `yaml`.
A `.yml` file uses the `yaml` format.

### Format rules

Some rules apply to every format, and each format then adds its own.

Every key becomes a secret whose name is the key.
When the key is also a valid environment variable name, Coder sets the secret's environment variable target to that same key.
Key order is preserved, and imported secrets get no description and no file target, both of which you can add later by editing the secret.
Every key must have a non-empty value.
A key with an empty value, such as `DEBUG=` or `{"DEBUG": ""}`, fails the entire import, and it's the most common import failure.
Duplicate keys are rejected rather than resolved in favor of the last one.

A dotenv file holds a single `KEY=VALUE` pair per line, and an optional `export` prefix is stripped.
Blank lines and full-line `#` comments are ignored.
Inline comments aren't stripped, so `PASS=abc#123` stores `abc#123`.

> [!IMPORTANT]
> An unquoted dotenv value keeps everything after the `=`, so `PASS=abc # note` stores `abc # note` and the secret is silently wrong.
> The same text after a quoted value is an error instead, so `PASS="abc" # note` fails with an "unexpected data after closing double quote" error.
> Put the comment on its own line above the key to keep it out of the value.

`$VAR` and `${VAR}` aren't expanded, and Coder stores the literal text.
That's deliberate, because secret values often contain `$`.
Unquoted values are trimmed of surrounding whitespace, unlike a value piped to `coder secret create`, which is read verbatim.
Single-quoted values are literal, so `'a\nb'` stores a backslash followed by an `n`.
Double-quoted values support the `\n`, `\t`, `\r`, `\\`, and `\"` escapes and keep any other escape sequence literal, so `"a\nb"` stores a newline.
Multiline values aren't supported, so use JSON or YAML for PEM keys, SSH keys, and certificates.

A JSON file holds a single top-level object that maps names to string values, with no trailing content after the object.
Every value must be a string, and numbers, booleans, `null`, nested objects, and arrays are all rejected.
Quote numeric and boolean values, and use `\n` inside a string for a multiline value:

```json
{
  "PORT": "8080",
  "TLS_KEY": "-----BEGIN PRIVATE KEY-----\nMIIEv...\n-----END PRIVATE KEY-----\n"
}
```

A YAML file holds a single document with a mapping at the top level, and multiple documents are rejected.
Keys and values must be strings, so quote numeric and boolean values.
Block scalars (`|` and `>`) are strings, which makes YAML a good choice for a multiline value such as a PEM key:

```yaml
PORT: "8080"
TLS_KEY: |
  -----BEGIN PRIVATE KEY-----
  MIIEv...
  -----END PRIVATE KEY-----
```

Aliases (`*name`) and merge keys (`<<:`) are rejected.
An anchor on a string value is imported as an ordinary secret, but no other entry can reference it.

### Secret names and environment variable names

A key has to be valid as a secret name, and separately as an environment variable name.
The two rules have different consequences.

Keys that contain `/`, `?`, or `#`, keys that are empty or only whitespace, keys with leading or trailing whitespace, and keys longer than 255 characters aren't valid secret names, and they fail the entire import.

Keys that are valid secret names but not valid environment variable names are imported without an environment variable target, so they aren't injected into any workspace.
A key isn't a valid environment variable name when it contains a character outside `A-Z`, `a-z`, `0-9`, and `_`, starts with a digit, is longer than 256 bytes, or is reserved.
Reserved names cover the core POSIX login, locale, and shell variables (`PATH`, `HOME`, `SHELL`, `USER`, and others), the XDG base directory variables such as `XDG_CONFIG_HOME`, and every name in the `CODER_`, `GIT_`, `LC_`, `LD_`, and `DYLD_` families.
The reserved check ignores case, so `path` is reserved as well as `PATH`.

Secrets imported without an environment variable target show **Not set** in the **Environment variable** column of the **Secrets** page, and they carry a **not injected** badge in the **Type** column.
Edit the secret and set a valid environment variable name or a file path to start injecting it.
The success toast also reports how many secrets were imported without an environment variable name, but the toast is transient, so read the table if you miss it.

Creating a single secret with a reserved environment variable name is rejected.
An import instead downgrades that key to a secret with no environment variable target.

### Limits and conflicts

An import is subject to the same [limits](#limits) as any other secret, plus a cap on the size of the file.

The file must be 1&nbsp;MiB or smaller.
The 50-secret cap is a total per user rather than a per-file cap, so the keys in the file plus the secrets you already own must not exceed 50.
A 50-key file therefore fails if you own any secrets at all.
The per-secret and combined byte caps apply as well, including the env-injected budget, because an import sets an environment variable target for every key that is a valid environment variable name.

Uniqueness is enforced separately on the secret name, the environment variable name, and the file path, and any conflict aborts the whole import.
Every colliding key is reported at once, so you don't have to re-import to find the next one.
A key can therefore collide with the environment variable of an existing secret that has a different name, and the fix is to change the other secret's environment variable target.
Secret names are case-sensitive, so importing `API_KEY` doesn't conflict with an existing `api_key`, and you end up with 2 secrets that hold the same value.

### Failed imports

An import is atomic.
If anything fails, Coder creates no secrets and writes no audit log entries.
Errors appear in the dialog, and the dialog stays open so you can fix the file and choose it again.

Errors come in 3 shapes:

- Parse errors name the line for a dotenv file, for example a line with no `=` or a duplicate key, and name the offending key for JSON and YAML.
- Per-entry validation and conflict errors use a field path such as `secrets[1].value`, and name the key, plus the line for dotenv and YAML.
- Limit errors name the key that tripped a byte cap, and report your remaining headroom when the file would push you past the 50-secret cap.

The index in a field path is zero-based and counts the entries the parser found, so it isn't a line number.
Blank lines and comments are skipped.
In this file, the empty value is reported as `secrets[1].value`, because `DEBUG` is the second entry even though it's on the third line:

```dotenv
# Application secrets
API_KEY=abc123
DEBUG=
```

### Import with the API

To import a file programmatically, send `POST /api/v2/users/{user}/secrets/batch` with a JSON body that contains `format` (`env`, `json`, or `yaml`) and `content` (the raw text of the file as a JSON string):

```json
{
  "format": "env",
  "content": "API_KEY=abc123\nDATABASE_URL=postgres://user:pass@db.internal/app\n"
}
```

The content isn't base64-encoded, and the endpoint isn't a multipart upload.
Everything else in this section applies to the API too.
For the response body and the status codes, refer to [Import user secrets from a file](../reference/api/secrets.md#import-user-secrets-from-a-file) in the Secrets API reference.

The rest of this guide covers the `coder secret` CLI, which creates, updates, and deletes secrets one at a time.
The same behaviors, limits, and injection rules apply however you manage a secret.

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

Refer to [How your secrets reach a workspace](#how-your-secrets-reach-a-workspace) for what happens to running workspaces when you delete a secret.

For full command details, refer to the [`coder secret` reference](../reference/cli/secret.md).
