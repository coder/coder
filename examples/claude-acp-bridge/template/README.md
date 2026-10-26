---
display_name: Native Agent Job
description: Start a native Claude Code, Codex, or OpenCode V2 session from a workspace prompt parameter.
icon: /icon/claude.svg
tags: [experimental, docker, agents]
---

# Native agent job workspace

Run a one-time task in a persistent Docker workspace using Claude Code, Codex,
or OpenCode V2. The Coder workspace agent owns the local native-session manager.
There is no separate Coder broker process or `coder-acp` executable.

This experimental template requires a Coder build with native-agent support and
the server's `native-agents` experiment enabled. The template enables
`CODER_AGENT_NATIVE_AGENTS=true` in the container environment before the agent
starts. Restrict access to the workspace and its credentials to their authorized
owner.

## Prerequisites

- A Linux Docker host reachable by the Terraform provisioner and a Coder access
  URL reachable from containers. Localhost bootstrap URLs are mapped to
  `host.docker.internal`.
- Authentication for the selected harness, provisioned as described below.
- Permission to use the harness and provider under their policies.

The image installs the official pinned packages:

| Harness | Package | Version |
| --- | --- | --- |
| Claude Code | `@anthropic-ai/claude-code` | `2.1.309` |
| Codex | `@openai/codex` | `0.164.0` |
| OpenCode V2 | `@opencode/cli` | `2.0.28` |

Node.js 22 remains in the image for native CLI packaging. OpenCode's V2 package
is not the V1 `opencode-ai` package. The image checks its reported version.
Coder's normal init script downloads the deployment's Coder binary into
`/opt/coder/bin`, which is on both the agent and workspace shell PATH.

## Authentication

Authentication files must be owned by workspace UID 1000, mode `0600`, within
private directories. Existing provider settings and credentials are never
silently replaced. Invalid or expired credentials require explicit native login
or reconciliation by the owner. Protect the persistent home volume and backups:
credentials, prompts, transcripts, and tool output can all contain secrets.

### Claude Code

Either provision native authentication under `/home/coder/.claude`, or set the
optional `claude_auth_file` Terraform variable to an absolute, owner-only JSON
file on the Docker host. The file is mounted read-only at
`/run/secrets/claude-auth.json`; do not upload it with the template.

Accepted mounted formats:

- Claude OAuth credentials containing `claudeAiOauth.accessToken`.
- An object containing `ANTHROPIC_API_KEY`.
- An object containing `ANTHROPIC_AUTH_TOKEN` and an HTTPS `ANTHROPIC_BASE_URL`
  without embedded URL credentials.

OAuth credentials and gateway settings are copied only when native settings and
credentials are absent. For an API key, Claude's `apiKeyHelper` invokes
`coder native-agent read-auth` to read the mount. This helper intentionally writes
only the key to stdout. Do not run it in startup scripts or logs.
The workspace owner and their native agents can read mounted credentials.

The exact generated helper `node /opt/native-agent/read-auth.mjs` is migrated to
`coder native-agent read-auth` during setup, preserving other settings. Custom
helpers and existing credentials are not replaced. This repair also runs before
a legacy initial job is rejected.

The Claude model parameter initializes new settings only. Existing settings take
precedence. Existing Claude configuration must already trust the project;
otherwise open Claude in the project and confirm trust yourself.

### Codex

`CODEX_HOME=/home/coder/.codex` retains native authentication, configuration, and
conversation history. Provision native login there, or put `OPENAI_API_KEY` in
the private JSON file selected by `claude_auth_file`. Despite its name, this
read-only mount supports Codex credentials, Claude credentials, or both together.
Only the host file path enters Terraform; the template never reads key contents
into variables or the agent manifest.

Setup passes a mounted OpenAI key to `codex login --with-api-key` on stdin,
without logging native output or forwarding the key in child environments.
An existing private `auth.json` is checked with `codex login status`, never
replaced, even if a different key is mounted. Do not put keys in workspace
parameters, prompts, shell arguments, or checked-in files.

If you previously supplied `codex_api_key` through this template, revoke or rotate
that key. Removing the variable does not erase historical Terraform state or
agent manifests, and marking a variable sensitive did not protect its encoded
value in the browser-visible agent environment.

### OpenCode V2

The dedicated server uses this isolated persistent home:

```text
/home/coder/.local/state/coder-native/opencode-home
```

Its `HOME` is that directory. `XDG_CONFIG_HOME`, `XDG_DATA_HOME`,
`XDG_STATE_HOME`, `XDG_CACHE_HOME`, and `TMPDIR` are respectively its `config`,
`data`, `state`, `cache`, and `tmp` subdirectories. Ordinary workspace-home
OpenCode configuration is not used.

Provision an authorized provider and model in
`config/opencode/opencode.jsonc` beneath this home using the
[V2 configuration](https://opencode.ai/v2/docs/config) and
[provider](https://opencode.ai/v2/docs/providers) formats. Provider keys can be
referenced from private sidecar files rather than embedded in the template.
Native connected accounts and sessions are retained in
`data/opencode/opencode.db`; do not edit or copy a live database.
Setup does not modify provider settings or preflight a model. If authentication
or a default model is unavailable, inspect the session and use the native TUI's
`/connect` and `/models` commands. No initial task is automatically retried.

## Publish and use

The template directory is self-contained; no adapter packaging step is needed.
From the repository root, publish under a new template name:

```sh
coder templates push native-agent-job \
  --directory examples/claude-acp-bridge/template
coder templates edit native-agent-job --private
```

For mounted Claude or Codex authentication, supply the host path through
`claude_auth_file` in the template variable workflow. Do not replace another
user's template or demo workspace.

Create a workspace and select the immutable **Native harness**, **Initial
prompt**, optional public HTTPS **Repository URL**, and **Claude model**.
Explicitly select **Trust this project** to allow unattended setup. This permits
loading project instructions and configuration, not bypassing native tool
approvals or sandbox permissions. Inspect the repository before trusting it.
Coder stores the prompt parameter, so do not include secrets.

The startup shell calls `coder native-agent configure --setup`. This prepares the
project and authentication, then saves the literal prompt in the private
`/home/coder/.local/state/coder-native/config.json`. It does not contact the
manager or start native inference. After startup scripts finish, the agent starts
the manager, restores saved sessions, and attempts the initial job once.
An identical configuration is a no-op apart from the exact generated Claude
helper repair; divergent configuration is rejected without overwriting the
existing file. No shell evaluates the prompt.

Select a session on the workspace page to observe it. **Open** runs
`coder native-agent attach SESSION_ID` in a terminal, connecting to the existing
native conversation. **Observe** returns control to automation without closing
the terminal. Native input or permissions may need attention before work can
continue. A dispatched task is not necessarily finished.

## Persistence and shutdown

New manager state lives under `~/.local/state/coder-native`, including the private
`manager.json` loopback descriptor and session history. Local CLI requests use
its bearer token; tokens and native attach environments are not printed by the
CLI or exposed through the browser API. Claude and Codex retain their native
homes. Existing `~/.local/state/coder-acp` data is neither migrated nor deleted.
If a legacy `job.json` exists, including an unreadable, malformed, or unknown-outcome
record, configuration refuses to create a new initial job. Ambiguous legacy paths
are also rejected. Inspect the retained native history and submit new work
explicitly; do not delete old records to make the immutable prompt run again.

Browser disconnection and agent observer shutdown do not stop native work.
The explicit workspace stop hook runs `coder native-agent shutdown` to stop
native services without deleting saved conversations. Forced container
termination can bypass this hook.

Restoration uses original native session IDs without replaying prompts. The
initial job is durably claimed before dispatch and never automatically retried,
including after an ambiguous outcome. Do not delete records to retry a task:
inspect native history and tool side effects first, then explicitly submit new
work. These guards do not guarantee exactly-once external side effects across
crashes. Choose an autostop policy that allows enough time for the task.
