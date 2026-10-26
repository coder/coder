---
display_name: Native Agent Job
description: Start a native Claude Code, Codex, or OpenCode V2 session from a workspace prompt parameter.
icon: /icon/claude.svg
tags: [experimental, docker, agents]
---

# Native agent job workspace

Create an isolated Docker workspace and submit an initial prompt to a native Claude Code background session, Codex app-server thread, or OpenCode V2 session.
The immutable **Native harness** parameter defaults to Claude Code; select Codex or OpenCode V2 when creating a new workspace.
The session appears on the workspace page automatically.
Select its session ID to observe the transcript, **Open** to use the native terminal, or **Observe** to return control to automation.
This experimental template requires the Coder `native-agents` experiment and the adapter from this checkout.

## Prerequisites

- A Linux Docker host reachable by the Coder Terraform provisioner.
- A Coder access URL reachable from containers.
  For a local deployment, the template maps `localhost` to `host.docker.internal` when bootstrapping the agent.
- For a Claude initial job, Claude authentication provisioned in a private JSON file on the Docker host, readable by UID 1000 and mode `0600`.
  The optional `claude_auth_file` variable mounts this file read-only, not uploaded with the template or embedded in image layers.
- For a Codex initial job, an authorized OpenAI API key supplied through the sensitive `codex_api_key` Terraform input variable, or existing native authentication in the persistent workspace home.
  Claude credentials are not required for Codex jobs.
- For an OpenCode initial job, an available model in its native saved configuration or connected provider accounts in the private persistent home.
  No Claude or Codex credential is required, and the template has no OpenCode key input.
- Permission to use the selected harness under the provider's policy.
  The image installs pinned native CLIs: Claude Code `2.1.309`, `@openai/codex` `0.164.0`, and `@opencode/cli` `2.0.28`, using Node.js 22.
  The [official V2 distribution](https://opencode.ai/v2/docs/) is `@opencode/cli`, not the V1 `opencode-ai` package.
  Its platform binary dependencies are pinned to `2.0.28`; the image build checks that `opencode --version` prints `opencode v2.0.28`.

### Claude authentication

Claude uses one administrator-supplied authentication file for the template.
Restrict template access to that credential's authorized owner.
For a shared deployment, replace the bind mount with per-user secret provisioning before granting access to other users.
The workspace user and their native agent can read mounted credentials.
Do not upload your home directory, credential file, or broker state with the template.

The authentication file accepts one of these formats:

- Claude's OAuth credentials JSON, containing `claudeAiOauth.accessToken`.
- A JSON object containing `ANTHROPIC_API_KEY`.
- A JSON object containing `ANTHROPIC_AUTH_TOKEN` and an HTTPS `ANTHROPIC_BASE_URL` for an authorized provider gateway.

OAuth credentials and gateway settings are copied to owner-only files in the workspace home; the API key is read from the mount by Claude's `apiKeyHelper`.
Refreshing or rotating credentials in an existing workspace requires explicit reconciliation.
The startup script does not overwrite existing Claude settings.

### Codex authentication and storage

The template sets `CODEX_HOME=/home/coder/.codex` in the persistent home volume.
This private directory retains native authentication (`auth.json`), configuration (`config.toml`), and session history across container replacements.
Protect the whole home volume and its backups: native transcripts and authentication can contain secrets.

For a first Codex job, startup supplies the optional API key to `codex login --with-api-key` on stdin, never as a command argument or in child process environments.
Login output is not printed.
An existing private `auth.json` is checked with `codex login status` and is never replaced, even when the key variable is supplied.
Without the key variable, native login must already be available in this home.
Startup does not copy authentication from the Docker host or modify Codex model, approval, or sandbox configuration.

The key is a sensitive template input variable, not a workspace parameter: `coder_parameter` does not support sensitive values, and cosmetic input masking does not protect provisioning logs.
Supply `codex_api_key` through your authorized template secret-input workflow; do not put it in prompts, command arguments, or checked-in files.
Coder stores this variable and Terraform retains it in state and the agent's bootstrap environment.
Base64 encoding protects it from environment-variable expansion; it is not encryption.
The variable is template-wide, so restrict this example template to the credential's authorized owner.
For shared templates, provision per-user native authentication instead.
Restrict access to the workspace and provisioning state.
For deployments that must not store an API key in Coder or Terraform, leave this variable empty and provision native authentication separately before the initial job.
Credential rotation and expired authentication require explicit native login; restarts do not provision credentials or resubmit the job.

### OpenCode authentication and storage

OpenCode uses its native saved provider configuration and accounts, without startup credential writes or provider/model overrides.
Provision only credentials authorized for this workspace's owner, outside the template bundle and image.
The template never reads OpenCode credentials from the Docker host or copies another user's configuration.

The dedicated OpenCode server and initial preflight share an isolated native home beside the broker descriptor: `dirname(stateFile)/opencode-home`.
For this template, `stateFile` is `/home/coder/.local/state/coder-acp/bridge.json`, so the persistent home volume retains:

| Native variable   | Persistent path                                           |
|-------------------|-----------------------------------------------------------|
| `HOME`            | `/home/coder/.local/state/coder-acp/opencode-home`        |
| `XDG_CONFIG_HOME` | `/home/coder/.local/state/coder-acp/opencode-home/config` |
| `XDG_DATA_HOME`   | `/home/coder/.local/state/coder-acp/opencode-home/data`   |
| `XDG_STATE_HOME`  | `/home/coder/.local/state/coder-acp/opencode-home/state`  |
| `XDG_CACHE_HOME`  | `/home/coder/.local/state/coder-acp/opencode-home/cache`  |
| `TMPDIR`          | `/home/coder/.local/state/coder-acp/opencode-home/tmp`    |

These are the native process's variables, not the ordinary workspace shell's HOME/XDG paths.
The lifecycle helper removes inherited OpenCode database, service, and inline-configuration overrides and disables automatic updates.
Neither preflight nor the dedicated server uses the ordinary `/home/coder/.config/opencode` configuration or shared database.

Create or provision `/home/coder/.local/state/coder-acp/opencode-home/config/opencode/opencode.jsonc` as UID 1000 with mode `0600` inside mode `0700` directories before the initial job.
Use the [V2 configuration](https://opencode.ai/v2/docs/config) and [provider](https://opencode.ai/v2/docs/providers) formats, preserving unrelated settings.
For example, provision a mode `0600` credential sidecar named `provider-key` beside that configuration file and reference it rather than putting a key in the template:

```jsonc
{
  "$schema": "https://opencode.ai/config.json",
  "model": "your-provider/your-model",
  "providers": {
    "your-provider": {
      "settings": {
        "apiKey": "{file:./provider-key}",
      },
    },
  },
}
```

Replace the provider and model IDs with ones supported by your account; custom gateways also require their native package, endpoint, and model configuration.
File provisioning is the simplest initial setup. For interactive setup before the broker exists, use the environment returned by `openCodeEnvironment(stateFile, env)` from the packaged `adapter/src/opencode-server.mjs`, create its private HOME/XDG/TMPDIR directories, and launch `opencode --standalone` from the project with that environment and a `0077` umask. Use `/connect` and `/models` there without submitting a task.
Do not run standalone setup in the ordinary workspace-home environment: it configures a different database.
Once a native session is running, select **Open** on its card and use `/connect` or `/models` in the TUI attached to the dedicated authenticated server.
OpenCode V2 stores connected accounts and native sessions in `/home/coder/.local/state/coder-acp/opencode-home/data/opencode/opencode.db`, not a new `auth.json`.
Do not edit or copy a live database to manage credentials; use native account management and protect the home volume and its backups.

Only an initial OpenCode job checks the effective default model, using the read-only `opencode api --standalone get /api/model/default` command from the trusted project directory.
Startup derives its exact native environment from the same `openCodeEnvironment` helper as the lifecycle, creates the private directories first, and uses the isolated configuration and database listed above.
This short-lived private server exits before the broker starts its dedicated server; it neither discovers the managed service nor changes provider configuration.
Native response bodies are suppressed because model settings can contain secrets.
If no model is available or configuration cannot load, startup reports setup instructions without submitting work.
Native defaults, including any available free models, remain OpenCode's choice; set an explicit native model when a particular provider is required.
After correcting a preflight failure, rerun startup; it may submit the initial job only if no dispatch record or broker was created.
Restarts with a retained job skip this check, leave credentials unchanged, restore sessions, and never retry the initial task.

## Publish the template

Package this checkout's adapter from the `examples/claude-acp-bridge` directory:

```sh
node scripts/package-template.mjs /absolute/new/native-agent-job
cd /absolute/new/native-agent-job
terraform init
terraform validate
coder templates push native-agent-job \
  --directory . \
  --variable claude_auth_file=/private/claude-auth.json
coder templates edit native-agent-job --private
```

For Codex or OpenCode initial jobs, omit `--variable claude_auth_file=...`.
Use a new destination directory each time you package an updated adapter.
Publish under a new template name when testing; do not replace another user's demo workspace or template.
The package excludes dependencies, tests, runtime state, and credentials.
The image installs dependencies from the lockfile and restores the plugin manifest that Coder's directory uploader would otherwise omit.
The first workspace build downloads and builds the image on the Docker host.

## Start a job

Create a workspace from **Native Agent Job** and set:

- **Native harness:** Claude Code (default), Codex, or OpenCode V2.
  This selection is immutable for the initial job.
- **Initial prompt:** the task to run once.
  Coder stores this parameter, so do not include secrets.
- **Repository URL:** an optional public HTTPS Git repository.
  Leave this empty to use a new `/home/coder/project` directory.
  The template checks out the repository's default branch without submodules.
- **Claude model:** Haiku, Sonnet, or Opus, used only for Claude jobs.
  Codex and OpenCode receive no model override and use their native configuration defaults.
- **Trust this project:** explicitly authorize the selected harness to load the project's instructions and configuration.
  Startup refuses to launch without this confirmation.
  This does not bypass native approvals or broaden sandbox permissions.
  Native trust or approval prompts can still require your attention; inspect the project before authorizing it.

Startup installs the launcher and explicitly runs `coder-acp serve --detach --codex --opencode`, enabling all three adapters on one broker descriptor.
The broker starts a dedicated Codex app-server and an authenticated, loopback-only OpenCode V2 server, not either harness's managed service.
Both servers are available for new sessions regardless of the initial harness selection; OpenCode provider/model preflight runs only for an initial OpenCode job.
It then runs the ACP job client in a detached process, adding `--harness codex` or `--harness opencode` for that selection and leaving the default Claude invocation unchanged.
The client initializes ACP, creates a native session, selects automation mode, and submits the prompt as data.
Startup reports whether dispatch began or the session needs native input.
A dispatched job is not necessarily finished, and its tools may still need approval.
The client remains connected to record a dispatched turn's outcome.
No browser needs to be open for the job to run.

On the workspace page, select the session to view streaming output.
Use **Open** if the harness needs a permission decision or you want to steer it directly.
Confirm **Run command** in the terminal.
For Claude, use `Ctrl+Z` to detach when finished; for Codex, the native remote client interacts with the same app-server thread.
For OpenCode, the native TUI connects to the same authenticated explicit server and original `ses_...` session; closing the TUI does not submit, clone, resume, or cancel work.
Use `/connect` there when native provider authentication needs attention.
**Observe** switches the session back to automation; it does not close an already open terminal tab.
Existing work continues during control changes.

### When native setup needs attention

For Claude, a project can request MCP server approval even when **Trust this project** is selected.
The native session appears in the list before ACP registration, with a **Needs attention** status and a reason.
Select **Open** on that session and confirm **Run command** to review the native dialog.
Approve or decline the request yourself; Coder does not approve it automatically.

If setup blocked the job, its status is `blocked` and the initial task was not submitted.
After resolving the dialog, detach with `Ctrl+Z`, select **Observe**, and submit your task explicitly.
The retained job record prevents automatic resubmission, including after a restart.
Opening the web PTY does not create or restart a session or replay the initial prompt.

## Job records and restarts

The Docker volume retains the project, Claude conversation files, Codex's `CODEX_HOME`, OpenCode's native XDG directories and database, and the private dispatch record at:

```text
/home/coder/.local/state/coder-acp/job.json
```

The record contains the opaque session ID, harness, input fingerprint, and last recorded dispatch status, not the prompt text.
Codex session IDs use `codex:<UUID>` and OpenCode session IDs use `opencode:ses_<alphanumeric>`; use the full, case-preserving ID rather than stripping its prefix.
Restoration passes the original native ID to the owning adapter, never translating between harnesses or creating a replacement identity.
Older records without a harness are treated as Claude jobs without rewriting them.
The transcript can contain the prompt and model output; treat the workspace home as private.

Startup runs on every boot, but a matching job record prevents another session or prompt from being submitted.
Different input or a different harness with the same record is rejected without overwriting it.
The input fingerprint algorithm is unchanged; harness identity is checked separately.
Keep the record even after failures: a disconnected client cannot prove that native work did not run.
Do not delete it to retry an ambiguous job.

Stopping the workspace terminates its processes.
The shutdown hook first asks Claude's native daemon to stop its workers and waits for their runtime registrations to be cleaned up, preserving saved conversations.
This applies to all Claude background jobs in this workspace, not only bridge-created jobs.
Forced container termination can bypass the hook. Stale records from a different PID domain still require manual inspection rather than unsafe automatic resume.
On the next start, the template restores the detached broker and explicitly starts dedicated native Codex and OpenCode servers so **Add session** works for all three harnesses without a terminal command.
Codex uses a private Unix socket beside the broker descriptor, rather than its managed, auto-updating daemon.
OpenCode uses its explicit authenticated local server, with private connection state rather than the user's shared background service.
Startup then automatically restores every eligible saved native session using its original identity, without sending a prompt or replaying an interrupted turn.
It reuses an authenticated live broker; after a verified container restart, it replaces only its own disconnected broker descriptor.
The private `broker-runtime.json` marker ties that descriptor to the previous container instance.
For older descriptors without a marker, startup migrates automatically only when both the saved listener and process are absent.
Authentication failures, live legacy PIDs, and ambiguous same-instance failures require inspection rather than automatic replacement.
A retained unknown job outcome remains visible but does not prevent broker startup or replay the task.

Broker startup reloads saved Claude session identities and retained transcripts from
`/home/coder/.local/state/coder-acp/bridge.json.sessions.json`.
Codex bridge records use `/home/coder/.local/state/coder-acp/bridge.json.codex-sessions.json`, with native history retained under `CODEX_HOME`.
OpenCode bridge records use `/home/coder/.local/state/coder-acp/bridge.json.opencode-sessions.json`, with native history retained in its database under `XDG_DATA_HOME`.
Startup restores all eligible saved sessions, not just the initial job, without needing a browser connection, waits for native readiness, and returns eligible sessions to automation.
Native readiness and pending work still gate new prompts.
Live sessions and pending registrations remain untouched. Each restoration is durably claimed once per container and broker instance before dispatch; failed or ambiguous attempts are not automatically repeated.
One failed restoration does not prevent other sessions from restoring.
Missing native transcripts require inspection. For Claude and Codex, queued work, goals, or surviving workers can prevent automatic native resumption; retained history remains readable.
OpenCode can restore read-only observation of queued or running work, but refuses new prompts until the native session is idle. It does not resume that work itself.
Choose **Open** only to launch a web PTY attached to a running session. It does not request restoration or restart the session, and remains available to open another terminal. **Observe** returns control to automation without closing any terminal.
The file is private because transcripts can contain prompts, tool output, and secrets.

Restoring sessions never resubmits interrupted prompts, and the retained job status may be stale.
Completed and failed sessions from older versions also appear when their bridge-owned native records remain on disk.
Those records recover session identities, not broker transcripts lost before persistence was enabled; the native terminal can still show its own saved conversation.
Inspect native conversation history and any tool side effects before continuing interrupted work.
The working directory defaults to `/home/coder/project`.
Choose an autostop policy that gives your job enough time to finish.
The dispatch guard prevents automatic resubmission, but does not guarantee exactly-once side effects across crashes.
