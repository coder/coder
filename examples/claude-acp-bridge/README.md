# Claude ACP workspace adapter (experimental)

Control a native Claude Code background session from the Coder UI, then join the
same session in Claude's terminal UI. This is an experimental adapter for an
existing Linux workspace, not a production-ready service. It requires a Coder
build with the `native-agents` experiment, or a standalone ACP client.

```text
Coder UI / ACP client
        |
        | ACP v1 over WebSocket to Coder (standalone clients use stdio)
        v
Coder backend: workspace SSH exec "coder-acp acp"
        |
        | ACP v1, NDJSON over stdio
        v
persistent authenticated loopback broker -> mod in native Claude
                                                    ^
                                                    |
Coder web terminal: coder-acp attach SESSION_ID -> claude attach
```

The mod uses `$.prompt.submit`, `$.turn.abort`, and structured turn/tool events.
There is no Agent SDK conversation, terminal scraping, or synthetic keyboard
input in the control path. Native Claude owns the conversation and execution.
An ordinary Coder workspace is sufficient; Coder Agents is not required.

## Prerequisites

- An already running Linux workspace. Install and start the broker as the same
  workspace user that runs the Coder agent, not as root or another account.
- Node.js 22 or later and npm. The installer captures the absolute Node executable
  path so non-interactive SSH does not depend on shell initialization or `nvm`.
- Claude Code with mods and agent view enabled. Runtime probing used 2.1.306
  and 2.1.309; these preview APIs are version-sensitive.
- Working Claude authentication and a trusted project directory. Run `claude`
  there once to complete login and trust if necessary. Managed policy must allow
  this mod. The bridge does not bypass trust, approve permissions, or configure
  authentication. Use the workspace's existing login or secret provisioning;
  do not copy credentials into the launcher, template, or logs.
- `claude` on the broker and terminal's `PATH`. The broker inherits its startup
  environment, not environment changes from later SSH clients.
- Keep the workspace running. Detachment survives SSH disconnect, not workspace
  shutdown, reboot, or a process manager that kills the workspace's processes.

## Install in an existing workspace

From a stable checkout of this directory:

```sh
cd /absolute/path/to/coder/examples/claude-acp-bridge
npm ci --ignore-scripts
./install.sh
export PATH="$HOME/.local/bin:$PATH"
coder-acp serve --detach --model haiku
coder-acp list
```

An empty list is expected. Installation, broker startup, and `list` do not launch
Claude sessions or make model requests. The installer does **not** run npm or
modify `package.json`, the lockfile, or existing dependencies. Use `npm ci
--ignore-scripts` for a reproducible fresh install; skip it when dependencies
are already installed and verified. Do not substitute an unpinned `npm install`
as a workspace startup step.

The generated owner-only executable at `~/.local/bin/coder-acp` delegates all
arguments to `src/cli.mjs`, using safely quoted absolute source and Node paths.
Keep both paths available. The source is not copied or packaged. Reinstall after
moving the source or removing the captured Node version. Installation refuses to
replace unrelated executables or symlinks.

Default state is `${XDG_STATE_HOME:-$HOME/.local/state}/coder-acp/bridge.json`.
The installer records this absolute path in the launcher, so every SSH client
uses the same broker regardless of its current directory or environment. State
has mode `700`; the descriptor and detached broker log have mode `600`.
The descriptor contains a bearer token controlling all broker sessions. Never
publish it or print it in startup diagnostics.

Optional installer flags:

```sh
./install.sh --help
# Alternative to separate install and serve commands; choose one startup method.
./install.sh --start --model haiku
# Custom paths, without starting anything:
./install.sh --bin-dir /your/bin --state-dir /your/private/state/coder-acp
```

`--start` is explicit, starts only a detached broker, and waits for its descriptor
to be ready. It does not automatically restart a broker, remove stale state, or
create a native session. `--model` configures that broker's future launches; it
is not stored in the launcher.

### Make the fixed SSH command available

The Coder backend runs exactly `coder-acp acp`; it does not know this checkout's
location or run installation. The web terminal runs `coder-acp attach SESSION_ID`.
Both commands must resolve for the Coder agent's non-interactive SSH environment.
An `export PATH` in your current terminal is useful for a local check but is not
a deployment configuration. Shell profile edits alone may not affect SSH exec.

Add the install directory and the directory containing `claude` to the Coder
agent's configured `PATH`, or install with `--bin-dir` into an existing writable
agent `PATH` directory. Preserve the workspace's other required paths. For
example, for a workspace user named `coder` with Claude in `~/.local/bin`, merge
this into the existing `coder_agent` resource:

```hcl
env = {
  PATH = "/home/coder/.local/bin:/usr/local/bin:/usr/bin:/bin"
}
```

Adapt the home and Claude locations to your workspace. After applying the
template change, verify `coder-acp list` through Coder SSH, not only an interactive
terminal. Do not put `serve` in a shell profile or start a broker for each client.

## Create a workspace that starts a job

Use the [Docker job template](template/README.md) to supply an initial prompt when you create a workspace.
The template prepares a private project, starts the broker, and dispatches the prompt through ACP without opening the workspace panel.
It keeps dispatch state in the workspace's persistent home directory so a restart cannot silently submit the same job again.
The template restores its detached broker after a verified container restart, allowing new sessions without automatically resuming old work.
This template-specific policy does not change the generic CLI's refusal to replace stale descriptors.
Native permissions still require terminal interaction when the configured policy asks for approval.

For an already configured workspace, save the prompt in an owner-only file and run:

```sh
coder-acp run --detach --cwd /absolute/project \
  --prompt-file /private/prompt.txt \
  --job-file /private/jobs/initial-job.json
```

The command returns a session identity after dispatch starts, not after the native turn finishes.
A detached ACP client observes completion and updates the private job record.
Load that session in the workspace panel or use `coder-acp attach SESSION_ID` to join it.
Without `--detach`, the client waits for the turn to finish.

Keep the job file for the lifetime of the job.
Repeated invocations with the same input return the recorded state immediately without creating another session or resending the prompt.
Foreground success requires completion; detached success requires a recorded dispatch attempt and `prompting` or `completed` status.
Earlier milestones return the JSON record with a nonzero exit status.
Native setup dialogs produce a `blocked` record with a session ID and `native_input_required`; the initial task was not submitted.
Select **Join** on the listed session to resolve its native dialog, then **Observe** and explicitly submit the task.
A matching job record is never automatically resumed or replayed after approval.
A transport failure can leave native work running; inspect the recorded session before taking further action.
This guard prevents automatic retries, but it does not recover a broker or promise exactly-once side effects across crashes.

## Workspace startup and shutdown

`scripts/workspace-startup.sh` is a runnable startup helper after installation:

```sh
/absolute/path/to/claude-acp-bridge/scripts/workspace-startup.sh --model haiku
```

It executes `~/.local/bin/coder-acp serve --detach`. Set `CODER_ACP_BIN` if the
launcher was installed elsewhere. The process gets a new OS session, closed
stdin, and redirected stdout/stderr, so it survives the startup script and SSH
connection exiting. It is not a supervisor and has no restart loop.

For an existing Coder template, a minimal addition is:

```hcl
resource "coder_script" "claude_acp" {
  agent_id           = coder_agent.main.id
  display_name       = "Start Claude ACP broker"
  run_on_start       = true
  start_blocks_login = true
  script             = <<-EOT
    #!/bin/sh
    set -eu
    exec /home/coder/.local/bin/coder-acp serve --detach --model haiku
  EOT
}
```

Install the source, dependencies, and launcher before this script runs. This is
a fragment for an existing Linux template, not a Docker image or a complete
provisioning template. It deliberately fails if a descriptor already exists.
A workspace restart after an unclean shutdown may require manual reconciliation.

Detached startup prints its PID and private log path, never the token. With the
default installation, inspect logs using:

```sh
tail -n 100 "${XDG_STATE_HOME:-$HOME/.local/state}/coder-acp/broker.log"
```

For foreground operation under a terminal or your own process manager:

```sh
coder-acp serve --model haiku
```

Use `Ctrl+C` to stop a foreground broker. For a detached broker, verify that the
PID reported at startup still belongs to this broker with `ps -p PID -o pid,args`,
then run `kill -TERM PID`. Do not signal an unverified PID from an old descriptor:
PIDs can be reused. Graceful shutdown removes only this broker's descriptor.
It **does not stop native sessions**. Use `coder-acp stop SESSION_ID` before broker
shutdown, or `claude agents --json --all` and `claude stop SHORT_ID` afterward.

### Missing, offline, or stale state

- **Missing descriptor:** start the broker explicitly. Clients fail rather than
  silently starting a new broker.
- **Offline, interrupted request, or rejected authentication:** check the private
  log, broker process, and which state file the launcher uses. A network failure
  can leave work running; do not blindly retry a launch or prompt.
- **Descriptor already exists:** a broker may still be running, or shutdown may
  have left stale state. Inspect the process and native sessions first. Only
  after confirming the old broker is stopped, remove the stale descriptor and
  explicitly start a new broker. No command here automatically performs recovery.

A missing native heartbeat makes `inspect` report `stale: true`. The broker
rejects new prompts, mode changes, and cancellations until that runtime responds,
but keeps observing accepted work. A valid heartbeat restores availability;
only matching, contiguous events establish a turn's outcome. A heartbeat gap
alone does not end the session or cause a prompt to be sent again. If a cancel
attempt fails while stale, reconnect ACP to observe the still-running turn and
join the native terminal to interrupt it once available. The new ACP connection
does not own the earlier prompt, and that cancellation is not automatically
retried.

Command acknowledgement deadlines are separate from heartbeat freshness. An
unacknowledged command or a broken event sequence still stops automation with an
unknown outcome. Inspect the native session before starting replacement work.

Broker restart replaces its port and token, but retains session identities and
bounded transcript history in `<descriptor-path>.sessions.json`.
Restored sessions are read-only archives: they remain listed as **Ended**, and
selecting one loads its retained transcript without polling or resuming work.
This marks the broker attachment as ended, not proof that an old native process
stopped. Native processes may survive a broker-only restart; inspect them natively
before creating replacements. There is no guarantee of exactly-once side effects
across crashes.

## Coder UI roundtrip

1. Enable `native-agents` in the Coder **server** experiments, for example
   `CODER_EXPERIMENTS=native-agents`, preserving any other required experiments.
   The workspace does not need this environment variable.
2. Install the adapter, configure the agent `PATH`, and explicitly start one broker as above, or create a workspace from the job template.
   Open your running Linux workspace as its owner.
   Coder discovers broker sessions automatically and shows their agent icon and session IDs.
   The experiment is unavailable in browser-only mode.
3. Select a session to observe its live transcript.
   You can also create an idle session in an absolute, already trusted project directory; sending a prompt requests model inference.
4. Send a text prompt.
   The UI receives ACP text, thinking, tool, mode, and completion updates over its WebSocket.
   Permissions and questions still require native Claude interaction; the adapter never approves them automatically.
5. Select **Join**, allow the new tab, and confirm **Run command** in Coder's terminal.
   Coder acknowledges human mode before running `coder-acp attach SESSION_ID` for that same session.
   Running work continues; new automation submissions pause.
6. Type a normal prompt or handle a permission in Claude.
   Use `Ctrl+Z` to detach to a shell while native work continues.
   The join command starts a login shell after attachment exits so Coder's reconnecting terminal does not reattach it.
7. Return to the workspace panel and select **Observe** to hand control back to automation.
   This does not close or detach a terminal tab that is still open.
   If ACP disconnected, select **Reconnect** and load the same session.
   Do not create a replacement merely because a tab disconnected.

### Agent identity and session diagnostics

ACP initialization reports the adapter's machine name, display title, and version in `agentInfo`.
This adapter identifies itself as `claude-acp-bridge` with the title **Claude Code**.
Coder uses known agent identities for local agent logos, not the configured model provider.
Unknown implementations retain their reported identity with a generic icon.

Session diagnostics are adapter metadata, not required ACP fields.
The session start time is the native background start time when discovery supplies it, or this broker's first registration; it is not the most recent heartbeat.
A broker without that timestamp shows an unknown start time rather than an invented age.
The adapter retains the latest known error summary even if a later turn succeeds.
Stale heartbeats are shown separately from integrity faults, and do not by themselves mean that native work failed.
The CLI broker persists identities, timestamps, error summaries, and retained transcript events in an owner-only file beside its descriptor.
Writes are atomic and synced before acknowledging recorded events. The file contains private conversation text; do not publish it.
Restored records expose `archived: true` and `ended: true`. Join, Observe, and new prompts are unavailable for archives.
Discovery also lists completed and failed bridge-owned native sessions, including those created before upgrading, as read-only archives.
Their identities can be recovered from native state, but transcripts not recorded by the persistence-enabled broker are not reconstructed.
Archives keep at most 100 sessions, 10,000 events or 8 MiB per session, and 32 MiB of event data in total, within a 40 MiB snapshot limit.
Older transcript events can be evicted; saved history is explicitly partial. Exhausting the session count refuses new launches rather than deleting session records.

Sessions waiting at a native startup dialog appear before the runtime registers.
They expose `registrationPending: true` and, when known, a controlled `blockedReason` in the adapter's ACP metadata.
Select **Join** to review the native prompt. Automation stays disabled until registration completes.
The first registration promotes the same native UUID rather than replacing the conversation.
Discovery uses structured Claude background-session state scoped to this broker's descriptor, not terminal output or unrelated native processes.

### Command contract

| Command                                                                               | Behavior                                                                                                                           |
|---------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------|
| `coder-acp acp`                                                                       | ACP v1 NDJSON on stdin/stdout. Diagnostics on stderr. Connects to the existing broker; no banners, native launches, or auto-start. |
| `coder-acp attach SESSION_ID`                                                         | Uses a full native UUID, selects human mode, and delegates to interactive `claude attach` with inherited terminal I/O.             |
| `coder-acp serve [--detach] [--model MODEL]`                                          | Explicitly starts a broker, foreground by default. Never overwrites an existing descriptor.                                        |
| `coder-acp list`                                                                      | Lists live sessions, owned pending sessions, and saved read-only archives as JSON.                                        |
| `coder-acp start --cwd /absolute/project`                                             | Explicitly creates an idle instrumented native session; returns its full ID.                                                       |
| `coder-acp run --cwd /absolute/project --prompt-file FILE --job-file FILE [--detach]` | Creates and prompts a session through ACP once, retaining private dispatch state.                                                  |
| `coder-acp inspect SESSION_ID`                                                        | Shows structured native identity, mode, runtime, and status.                                                                       |
| `coder-acp control SESSION_ID human\|automation`                                      | Changes control mode after native acknowledgement.                                                                                 |
| `coder-acp stop SESSION_ID`                                                           | Stops that native session, not the broker.                                                                                         |

All commands accept `--state /absolute/path/to/bridge.json`; use the same file for
broker and clients. ACP supports `session/new`, `session/list`, `session/load`,
`session/prompt`, `session/cancel`, and `session/set_mode`. Load requires the full
native UUID and matching project `cwd`. Only text prompts are supported.

**Disconnect is not cancellation.** Closing the ACP/WebSocket/SSH connection or
detaching the terminal does not stop native work or the broker. Explicit ACP
`session/cancel` targets the active prompt owned by that connection, not an
unrelated human turn. Cancellation cannot undo tool side effects. Switching to
human mode prevents new automation prompts; it does not interrupt running work.

Inside native Claude:

- `/bridge-status`: inspect bridge identity and control mode.
- `/bridge-human`: pause new automation prompts without cancelling current work.
- Normal native input also selects human mode.
- `/bridge-auto`: allow automation prompts again when idle.
- `Ctrl+Z`: detach to the shell while Claude keeps running.

A launch runs the mod's `/bridge-status` command instead of issuing an initial
model request. If registration times out, inspect `claude agents --json --all`
before retrying: a failed response does not prove that no session was launched.

## Standalone POC use

The source CLI still works without installing a launcher:

```sh
node src/cli.mjs serve --detach --model haiku
node src/cli.mjs list
node src/cli.mjs start --cwd /absolute/path/to/trusted/project
node src/cli.mjs attach SESSION_ID
```

Its default descriptor is this checkout's private `.runtime/bridge.json`, not
the installed launcher's state directory. To connect the installed launcher to
an existing POC broker, install with `./install.sh --state-dir "$PWD/.runtime"`.
Do not start another broker. Otherwise, keep source CLI and launcher invocations
on their respective state files, or pass an explicit matching `--state`.

Configure a standalone ACP client with the installed executable:

```json
{
  "command": "/home/coder/.local/bin/coder-acp",
  "args": ["acp"]
}
```

## Acceptance testing

```sh
npm test
npm run test:mod
npm run validate:mod
```

`npm test` exercises real ACP SDK framing, simulated runtimes, real HTTP brokers,
and isolated installer/launcher tests. Installer tests include spaces and
apostrophes, argument delegation, private permissions, token-safe diagnostics,
startup without a native launch, detached process lifetime, and refusal to
replace running or stale descriptors. `npm run test:mod` runs the real hooks
module in Claude's native offline test kit with mocked host APIs and a controlled
clock. Neither suite makes model requests. Job tests cover ACP dispatch,
concurrent duplicate prevention, ambiguous outcomes, detached lifetime, and
private records. Template tests cover packaging, literal parameter handling,
authentication provisioning, and restart guards. Validation can warn about the
missing marketplace icon; this local adapter is not a marketplace submission.

The opt-in live suite launches the actual broker, ACP subprocesses, native
Claude, and a test terminal. It makes billed inference requests, requires
Python 3 on Linux, and stops the sessions it creates:

```sh
BRIDGE_LIVE_CWD=/absolute/path/to/trusted/project npm run test:live
```

`BRIDGE_LIVE_MODEL` overrides the default `haiku`. Test artifacts use uniquely
named directories under `/tmp/opencode`; create that parent if running elsewhere.
The test-only PTY helper recognizes the prompt glyph to synchronize keystrokes.
It asserts behavior through structured bridge events and native process IDs,
not by reconstructing model answers from terminal output.

| Coverage                                                        | Test layer       |
|-----------------------------------------------------------------|------------------|
| Text streaming, real turn completion, tools                     | Offline and live |
| Native typing, takeover, detach, process continuity             | Live             |
| ACP disconnect, background completion, reconnect                | Offline and live |
| Cancellation correlated to the correct native turn              | Offline and live |
| Lost responses, duplicate commands, prompt drops                | Offline          |
| Human drafts, queued cancellation, subagent isolation           | Offline          |
| Authentication, stale runtimes, event gaps, bounded buffers     | Offline          |
| Installation, detached startup, stale descriptors, private logs | Offline          |
| Permission decisions, dialogs, keyboard ergonomics              | Manual native UI |
| Native conversation forks and filesystem isolation              | Not covered      |

The five standalone live scenarios passed against Claude Code 2.1.309 on
October 23, 2026. A separate browser check of this Coder integration also passed:

- Created and prompted a native session from the workspace panel.
- Joined Coder's browser terminal and sent native keyboard input; the reply
  appeared in both the native terminal and the workspace panel.
- Detached to a shell during a native Bash tool call, reconnected ACP before the
  turn completed, and received the completion in the panel.
- Switched back to Automation and sent another prompt from Coder.
- Cancelled a Coder prompt and verified the native turn reported `aborted`.
- Suspended the native test process for 12 seconds after prompt acceptance. The
  broker reported temporary staleness without faulting the session; after
  resuming the process, the original turn completed without resubmission.

Structured broker events and native process inspection confirmed that the
session UUID, native PID, and mod runtime ID stayed unchanged. This check used
a separate local Coder development database and a real workspace-agent process
on the existing development workspace filesystem, not a newly provisioned VM.
The Node/Claude installation and provider authentication were already present.
Repeat the browser check on the deployment and template you plan to use.

For a manual UX check, attach while Claude is busy, inspect the transcript,
interrupt if desired, send a correction, handle a normal permission prompt,
then run `/bridge-auto` and detach. This checks the human experience, not just
protocol correctness.

## Deliberate limits

- **Workspace adapter, not provisioning.** Source checkout, Node/Claude installs,
  authentication, trust, and workspace lifecycle policy remain template concerns.
- **One trusted workspace owner.** All clients share one token; there are no
  controller leases, client identities, or cross-user security boundaries.
  Human mode gates bridge prompts, not every possible native source of work.
- **Permissions and questions stay native.** Permission waits are observed where
  the hook reports them, but remote approval and every possible dialog are not
  implemented. Existing Claude permission configuration still applies.
- **Scoped discovery, not arbitrary process control.** Owned background sessions
  can be listed and joined before loading the mod. Automation requires this mod
  and its bridge descriptor; unrelated native processes are not adopted.
  New background launches pass settings explicitly because the supervisor does
  not inherit arbitrary caller environment variables.
- **Bounded saved history.** The CLI broker persists its retained current-runtime
  events, not complete saved Claude history. Metadata labels this partial history.
  Restored archives are read-only and can start after older events were evicted.
  Live clients still fail explicitly on expired cursors.
- **Restart is not seamless recovery.** Mod reload or native process replacement
  invalidates pending requests. Explicit reattachment starts a new history
  boundary. Broker restart needs newly configured sessions; it does not recover
  its old port, token, or pending work. No ambiguous prompt is automatically retried.
- **Native lifecycle still applies.** Claude can evict unpinned idle sessions;
  reattachment after eviction is a history resume, not process continuity.
  `/clear`, `/resume`, and forks require explicit reconciliation or a new bridge
  session. A fork does not clone running subprocesses or guarantee file isolation.
- **Visible correlation marker.** Bridge prompts include a UUID marker and native
  mod attribution. This adapter does not claim a runtime-provided request ID.
- **Bounded telemetry.** Oversized events or exhausted buffers stop automation
  rather than silently dropping output. Native work can still continue; attach
  to reconcile it.

## Sources

- [Claude mods API](https://code.claude.com/docs/en/plugins/mods/api.md)
- [Native offline mod tests](https://code.claude.com/docs/en/plugins/mods/test.md)
- [Native background sessions and attachment](https://code.claude.com/docs/en/agent-view.md)
- [Agent Client Protocol](https://agentclientprotocol.com/)
