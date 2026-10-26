# Native workspace agent reference

Use this reference to understand and operate the experimental Go workspace-agent integration.
For the three-harness diagram, refer to the [overview](README.md).
For a runnable workspace, use the [Docker template](template/README.md).

## Enablement

The deployment requires the `native-agents` experiment.
The Linux workspace agent also requires `CODER_AGENT_NATIVE_AGENTS=true` before its process starts.
This isn't a Coder Agents or `chatd` integration.

Install and authenticate the native harnesses as the same OS user that runs the workspace agent.
The template pins Claude Code to `2.1.309`, Codex to `0.164.0`, and OpenCode V2 to `2.0.28` from `@opencode/cli`.
OpenCode V1 from `opencode-ai` isn't compatible.
Complete native project trust and provider setup before unattended work.
Authentication is separate for each harness; selecting an initial harness doesn't authenticate the others.
The integration doesn't approve native permission dialogs.

The template prepares the project, credentials, native installations, and initial-job configuration.
The Go manager starts after the workspace's startup scripts finish.
It restores saved conversations before attempting the configured initial job.
Native subprocesses receive the workspace user's prepared environment, not a later browser or SSH client's environment.

Before accepting sessions, the manager prepares prerequisite directories for all three harnesses, regardless of the initial harness:

- Claude: `$HOME/.claude`.
- Codex: `$CODEX_HOME`, or `$HOME/.codex` when unset or empty.
- OpenCode: `opencode-home` within the manager's state directory, including its `config`, `data`, `state`, `cache`, and `tmp` directories.

The template's `configure --setup` command also prepares these directories on every startup, including when it retains an existing job configuration.
Missing directories are created with mode `0700`; unsafe existing paths are rejected without changing their permissions or contents.
Directory preparation doesn't authenticate harnesses, start native processes, or change existing configuration and conversation files.

## Process and protocol boundaries

| Component          | Responsibility                                                                                    |
|--------------------|---------------------------------------------------------------------------------------------------|
| Browser panel      | ACP client, Markdown transcript, separate thinking, bounded tab-scoped drafts and selection       |
| `coderd` relay     | Existing authentication, SSH authorization policy, origin checks, and workspace connection        |
| Go workspace agent | ACP projection, native adapters, session discovery, event journal, dispatch guards, and lifecycle |
| Claude mod         | Structured events and native prompt/control APIs inside Claude's JavaScript runtime               |
| Native harnesses   | Model calls, tool execution, native approvals, and authoritative conversations                    |
| Local attach CLI   | Authenticated session lookup and attachment to the existing native TUI                            |

The browser connects to `/api/v2/workspaceagents/{agent}/experimental/acp`.
The relay connects to `/api/v0/native-agents/acp` over the existing agent API on tailnet.
Both WebSockets carry ACP v1 newline-delimited JSON in binary frames.
There is no SSH exec in this path and no per-connection adapter process.

ACP isn't the native Codex or OpenCode protocol.
The Go adapters translate stateful native observations and operations into the shared session contract.
Claude exchanges structured events and commands with its embedded mod over authenticated loopback HTTP.
Codex uses a dedicated app-server's native RPC; OpenCode uses a dedicated authenticated HTTP/SSE server.
None of these paths scrape a terminal or run a second model loop.

The local manager descriptor contains a loopback address and bearer token.
The local API serves the attach CLI and Claude's mod; it isn't mounted on the remote agent API.
Native attachment credentials stay in the local process environment rather than the browser or terminal URL.
Keep the descriptor and native configuration private.

## Session lifecycle

- **Create:** starts a new native conversation and records its native identity and working directory.
- **Load:** replays retained agent events and follows new observations. It never resumes native execution or submits a prompt.
- **Open:** opens another Coder web PTY and runs `coder native-agent attach SESSION_ID`. The command selects human control and verifies the existing native conversation before attachment. It doesn't restore a stopped session.
- **Observe:** permits future automation prompts. It doesn't close an attached terminal or cancel a running turn.
- **Disconnect:** releases that ACP observer. Native execution doesn't depend on the browser remaining connected.
- **Agent restart:** releases observation without intentionally stopping native services, then reconnects to eligible saved conversations.
- **Workspace restart:** startup restores eligible saved native identities without resubmitting earlier tasks. Restoration failures are independent.
- **Workspace stop:** the template explicitly asks the manager to stop owned native processes. Persistent history and credentials remain on the workspace volume.

Human and automation modes gate input from this integration.
They aren't an exclusive lease against direct native terminal input.
Native permission dialogs remain terminal-only.
Unsupported cancellation is reported rather than broadened to unrelated native work.

Claude receives your literal prompt without plugin attribution or request markers in model context.
Request tracking uses native receipt metadata and turn events.
Unresolved competing submissions block further automation rather than risk stopping unrelated work.
Dropped or rewritten submissions, indistinguishable queued prompts, and attributed prompts from other plugins remain blocked until the native session restarts.
Native terminal access remains available.

Claude restoration uses the same native conversation ID, not a newly seeded conversation.
Codex restoration checks native thread state before resuming it.
Do not edit a cold Codex thread's native queue or goals concurrently with restoration.
Codex `0.164.0` has no atomic check-and-resume operation: a concurrent native edit can start work between the check and resume.
This POC detects observed conflicts but cannot guarantee no automatic execution in that race.
OpenCode restoration reconciles saved native history; it doesn't use managed-service startup that could automatically continue interrupted work.
The native server and TUI use the same isolated OpenCode home.

## Persistence and no-replay safeguards

The Go POC uses `~/.local/state/coder-native`.
It doesn't migrate or overwrite the earlier JavaScript broker's `~/.local/state/coder-acp` directory.
Existing native histories and old demo workspaces aren't automatically imported.
A retained legacy initial-job record blocks new automatic job configuration, even when its outcome is unknown.
Use a new workspace for this POC rather than deleting old state to bypass that safeguard.
Treat the POC's on-disk schema and local protocol as experimental.

The manager persists native bindings, a bounded event journal, adapter observation receipts, and dispatch tombstones.
It records a dispatch attempt before calling a native mutation.
The initial job has its own durable attempt guard, including failures whose outcome can't be established.
An uncertain outcome never authorizes automatic resubmission.

Native history and the agent journal are different stores.
The journal supports a retained browser transcript, not a complete backup or reconstruction of every native message.
OpenCode reconciliation is bounded to 200 messages and 4 MiB and refuses to invent continuity when required anchors are missing.
Don't delete receipts or job records to make an uncertain operation retry.
No-replay safeguards don't guarantee exactly-once tool side effects.

The browser stores bounded draft and panel state in tab-scoped `sessionStorage`, keyed by authenticated user, workspace, and stable agent name.
Selection is restored only after discovery matches the native identity and working directory.
A restored draft is never sent automatically.
Storage can be unavailable or cleared by the browser, so it isn't durable conversation storage.

## Local operations

Run these commands in the workspace as its owner:

```sh
coder native-agent attach SESSION_ID
```

Replace `SESSION_ID` with the full identifier from the browser's native sessions panel, including the `codex:` or `opencode:` prefix when present.
`attach` opens the native terminal, or returns diagnostics if identity, readiness, or ownership checks fail.
Repeated attachment doesn't create another conversation.

For startup configuration options, run:

```sh
coder native-agent configure --help
```

The template invokes configuration before the manager starts.
Configuration doesn't itself execute the task.
To stop native services intentionally, use:

```sh
coder native-agent shutdown
```

Shutdown preserves saved state and only signals processes whose ownership the adapter can establish.
Ambiguous process ownership requires inspection rather than broad process termination.
Closing a browser tab or restarting the agent isn't equivalent to this command.

## POC boundaries

- Native preview APIs and lifecycle details remain version-sensitive.
- New native operations need harness-specific identity, ownership, and recovery checks; the common interface doesn't make those semantics identical.
- Conversation forking, browser-native approval dialogs, full native-history import, and cost attribution aren't implemented.
- The opt-in agent API inherits workspace connectivity authorization; it isn't a separate organization-scoped task service.
- A Coder Agents integration would need an organization-scoped task binding and execution-backend selection before generation, not a native harness wrapped as a model provider or tool inside another inference loop.

The Go migration removes the standalone Node broker and ACP processes, not the native executables.
Claude still requires its JavaScript mod, and native CLI packaging may require Node.

## Implementation and checks

- [Session manager, persistence, ACP, and native adapters](../../agent/x/agentnative/)
- [Workspace agent lifecycle](../../agent/agent.go) and [agent API](../../agent/api.go)
- [Authenticated deployment relay](../../coderd/workspaceagents_acp.go)
- [Local command implementation](../../cli/nativeagent.go)
- [Template setup](template/README.md)

Run the targeted Go suite from the repository root:

```sh
go test ./agent/x/agentnative
```

It checks persistence, dispatch guards, native protocol projection, cancellation, restoration, and ACP behavior using controlled native transports.
Live verification must also exercise the pinned executables: startup once, native TUI continuation, Observe while attached, browser reconnection, and workspace rebuild without task replay.
Unit tests alone don't establish compatibility with a different upstream release.
