---
display_name: Native Agent Job
description: Start a native Claude background session from a workspace prompt parameter.
icon: /icon/claude.svg
tags: [experimental, docker, agents]
---

# Native agent job workspace

Create an isolated Docker workspace and submit an initial prompt to a native Claude Code background session.
The session appears on the workspace page automatically.
Select its session ID to observe the transcript, **Join** to use the native terminal, or **Observe** to return control to automation.
This experimental template requires the Coder `native-agents` experiment and the adapter from this checkout.

## Prerequisites

- A Linux Docker host reachable by the Coder Terraform provisioner.
- A Coder access URL reachable from containers.
  For a local deployment, the template maps `localhost` to `host.docker.internal` when bootstrapping the agent.
- Claude authentication provisioned in a private JSON file on the Docker host, readable by UID 1000 and mode `0600`.
  The file is mounted read-only, not uploaded with the template or embedded in image layers.
- Permission to use Claude's mods and background sessions under the provider's policy.
  The image pins Claude Code `2.1.309` and uses Node.js 22.

This example uses one administrator-supplied authentication file for the template.
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

Use a new destination directory each time you package an updated adapter.
The package excludes dependencies, tests, runtime state, and credentials.
The image installs dependencies from the lockfile and restores the plugin manifest that Coder's directory uploader would otherwise omit.
The first workspace build downloads and builds the image on the Docker host.

## Start a job

Create a workspace from **Native Agent Job** and set:

- **Initial prompt:** the task to run once.
  Coder stores this parameter, so do not include secrets.
- **Repository URL:** an optional public HTTPS Git repository.
  Leave this empty to use a new `/home/coder/project` directory.
  The template checks out the repository's default branch without submodules.
- **Claude model:** Haiku, Sonnet, or Opus.
- **Trust this project:** explicitly authorize Claude to load the project's instructions and configuration.
  Startup refuses to launch without this confirmation.
  This does not bypass tool permissions.

Startup installs the launcher, starts the broker, and runs the ACP job client in a detached process.
The client initializes ACP, creates a native session, selects automation mode, and submits the prompt as data.
Startup reports whether dispatch began or the session needs native input.
A dispatched job is not necessarily finished, and its tools may still need approval.
The client remains connected to record a dispatched turn's outcome.
No browser needs to be open for the job to run.

On the workspace page, select the session to view streaming output.
Use **Join** if Claude needs a permission decision or you want to steer it directly.
Confirm **Run command** in the terminal, then use `Ctrl+Z` to detach when finished.
**Observe** switches the session back to automation; it does not close an already open terminal tab.
Existing work continues during control changes.

### When native setup needs attention

A project can request MCP server approval even when **Trust this project** is selected.
The native session appears in the list before ACP registration, with a **Needs attention** status and a reason.
Select **Join** on that session and confirm **Run command** to review the native dialog.
Approve or decline the request yourself; Coder does not approve it automatically.

If setup blocked the job, its status is `blocked` and the initial task was not submitted.
After resolving the dialog, detach with `Ctrl+Z`, select **Observe**, and submit your task explicitly.
The retained job record prevents automatic resubmission, including after a restart.
Joining does not create a new session or replay the initial prompt.

## Job records and restarts

The Docker volume retains the project, Claude conversation files, and private dispatch record at:

```text
/home/coder/.local/state/coder-acp/job.json
```

The record contains the session ID, input fingerprint, and last recorded dispatch status, not the prompt text.
The workspace's **Native job** metadata shows that recorded status, not a live liveness check.
The transcript can contain the prompt and model output; treat the workspace home as private.

Startup runs on every boot, but a matching job record prevents another session or prompt from being submitted.
Different input with the same record is rejected.
Keep the record even after failures: a disconnected client cannot prove that native work did not run.
Do not delete it to retry an ambiguous job.

Stopping the workspace terminates its processes.
On the next start, the template restores the detached broker so **Add session** works without a terminal command.
It reuses an authenticated live broker; after a verified container restart, it replaces only its own disconnected broker descriptor.
The private `broker-runtime.json` marker ties that descriptor to the previous container instance.
For older descriptors without a marker, startup migrates automatically only when both the saved listener and process are absent.
Authentication failures, live legacy PIDs, and ambiguous same-instance failures require inspection rather than automatic replacement.
A retained unknown job outcome remains visible but does not prevent broker startup or replay the task.

Broker startup reloads saved session identities and retained transcripts from
`/home/coder/.local/state/coder-acp/bridge.json.sessions.json`.
Select a stopped session to read its retained history without starting work.
Choose **Join** to explicitly resume the same saved native conversation in human mode, then **Observe** to allow new automation prompts after registration.
Resuming does not resend the original task or start other sessions. Missing native transcripts, queued prompts, and surviving workers require inspection instead of automatic replacement.
The file is private because transcripts can contain prompts, tool output, and secrets.

Startup alone never resumes interrupted jobs, and the retained job status may be stale.
Completed and failed sessions from older versions also appear when their bridge-owned native records remain on disk.
Those records recover session identities, not broker transcripts lost before persistence was enabled; the native terminal can still show its own saved conversation.
Inspect native conversation history and any tool side effects before continuing interrupted work.
The working directory defaults to `/home/coder/project`.
Choose an autostop policy that gives your job enough time to finish.
The dispatch guard prevents automatic resubmission, but does not guarantee exactly-once side effects across crashes.
