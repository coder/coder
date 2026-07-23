# Claude Code ACP adapter spike (Phase 0)

Date: 2026-07-09. Probe: throwaway Go program using github.com/coder/acp-go-sdk
v0.13.5, spawning the adapter via `node dist/index.js` with stdin/stdout pipes
(same semantics as the `Process` interface in transport.go). Live turns ran
against the dev.coder.com ai-gateway (`ANTHROPIC_BASE_URL`, gateway token as
`ANTHROPIC_API_KEY`) with `ANTHROPIC_MODEL=claude-haiku-4-5`.

## Pinned versions

| Component | Version |
|---|---|
| @zed-industries/claude-code-acp | 0.16.2 (npm dist-tag latest) |
| @anthropic-ai/claude-agent-sdk (bundled Claude Code) | 0.2.44 |
| @agentclientprotocol/sdk (adapter side) | 0.14.1 |
| github.com/coder/acp-go-sdk (chatd side) | v0.13.5 |
| node / npm | v22.19.0 / 10.9.3 |

## Capability matrix (initialize response, protocolVersion 1)

| Capability | Observed |
|---|---|
| agentCapabilities.loadSession | true |
| sessionCapabilities.resume | advertised (`{}`) |
| sessionCapabilities.fork / list | advertised (`{}`) |
| promptCapabilities | embeddedContext: true, image: true (no audio) |
| mcpCapabilities | http: true, sse: true |
| authMethods | one: id `claude-login` ("Run `claude /login` in the terminal") |
| agentInfo | name @zed-industries/claude-code-acp, version 0.16.2 |

session/new returns `modes` (currentModeId `default`; available: `default`,
`acceptEdits`, `plan`, `dontAsk`, `bypassPermissions`) and no `configOptions`.
The `acceptEdits` mode id used by this package exists. Authentication was never
needed: `ANTHROPIC_API_KEY` in the process env was sufficient, no
`authenticate` call.

## Findings

1. Stdio purity: PASS. Raw stdout captures from all five probe runs contain
   only newline-delimited JSON-RPC 2.0 frames, no banners or stray bytes.
   stderr stayed empty on healthy runs. It receives logs only on faults, e.g.
   an unhandled `$/cancel_request` notification (method not found, harmless)
   and an EPIPE stack when the process is killed.
2. Env handling: PASS. With `ANTHROPIC_BASE_URL` pointed at a local dummy HTTP
   listener, the adapter sent all API traffic there: `POST /v1/messages` and
   `POST /v1/messages/count_tokens`, 106 requests as it retried the dummy 500s.
   The adapter honors the env var and routes through it exclusively. With the
   real gateway URL, live turns completed normally.
3. Resume HARD GATE: PASS. Turn 1 on a fresh process ("remember the word
   pineapple") -> `SIGKILL` the adapter -> spawn a brand-new process ->
   `session/resume` with the same session id and cwd succeeded -> turn 2 ("what
   word did I ask you to remember?") replied "pineapple". `stopReason:
   end_turn` on both turns. session/resume does not replay history as
   session/update notifications (only session/load does), matching the
   suppression logic in establishSession.
4. Session storage: sessions persist under
   `~/.claude/projects/<cwd-slug>/<sessionId>.jsonl`, keyed by cwd. This
   confirms resume requires the same home directory and the same cwd, which is
   why TurnInput carries SessionCwd.
5. Cancellation: PASS. `session/cancel` sent mid-stream resolved the in-flight
   prompt in ~8ms with `stopReason: "cancelled"` and no RPC error. Note the
   wire value is `cancelled` (double l); acp-go-sdk's `StopReasonCancelled`
   matches.
6. Usage reporting: NOT PROVIDED. `PromptResponse.usage` was absent on every
   turn and no usage or context-window session updates were observed. Update
   types seen across all probe runs: `agent_message_chunk` and
   `available_commands_update`. Context compaction is not surfaced
   as an event (a `/compact` slash command is advertised, nothing automatic
   was observed on these tiny turns). chatd cannot bill by token from ACP
   data with this adapter version.
7. Latency (single workspace-class Linux host, warm npm cache): spawn +
   initialize ~0.2-0.4s; session/new ~1.7s; session/resume ~1.9s (spawn + init
   + resume ~2.1s); short haiku prompt turn 5-10s wall. Per-turn process
   respawn adds roughly 2s overhead, acceptable for chat turns.
8. Model pinning: `ANTHROPIC_MODEL` is NOT honored in ACP mode (found in
   dogfood 2026-07-09, full-stack dev instance). The env var demonstrably
   reaches the adapter and the bundled `cli.js` (verified via
   `/proc/<pid>/environ` during live turns), and the gateway serves the
   requested model when asked directly, but the agent SDK spawns `cli.js`
   in stream-json mode with no `--model` flag and that mode resolves the
   model from options/settings only; every ACP turn used the sonnet
   default. The same `cli.js` honors `ANTHROPIC_MODEL` in `-p` print mode.
   The runtime config `Model` knob is therefore ineffective with adapter
   0.16.2. Working alternative, verified end to end: the template writes
   `~/.claude/settings.json` with `{"model": "..."}`; `cli.js` runs with
   `--setting-sources user,project,local` and the next ACP turn served the
   pinned model.

## Limitations (not tested here)

- SSH-exec stdio purity on a real workspace agent. Covered by the branch's
  SSHTransport (non-PTY exec channel) plus the runtime preflight; a PTY or a
  noisy shell profile could still corrupt framing and must be validated in
  integration.
- Workspace stop/start and rebuild variants. Resume depends on
  `~/.claude/projects/...` surviving; that holds only when the home volume
  persists. Rebuilds that wipe home lose sessions, which is what the
  ReseedContext fallback exists for.
- Auth flows other than `ANTHROPIC_API_KEY` env (claude-login was not
  exercised).
- Gateway behavior under load, rate limiting, and long multi-tool turns.

## Security note: the provider key is exposed to the workspace owner

The per-turn env injection means `ANTHROPIC_API_KEY` lives in the adapter
process environment inside the chat owner's workspace. Workspace owners run
code as the same user and can read `/proc/<pid>/environ`, so enabling the
runtime hands the configured key to every user who can create runtime chats,
letting them call the provider directly and bypass chatd usage limits.
Admins must configure a scoped, rate-limited key (for example an AI gateway
token), never a raw organization-wide provider key. Keeping the key
server-side requires routing turns through a coderd-side proxy with
short-lived per-user credentials, which is follow-up work.

## Verdict

GO. Resume across process kills works via session/resume with the same cwd,
and ANTHROPIC_BASE_URL routing is fully honored, so the per-turn process model
and gateway routing assumptions in this package hold for adapter 0.16.2.
Two gaps remain with this adapter: usage/billing data is not available over
ACP, and ANTHROPIC_MODEL is ignored in ACP mode (finding 8), so model pinning
requires the template to write `~/.claude/settings.json`.

## Addendum: adapter renamed, re-validated on 0.58.1 (2026-07-09)

`@zed-industries/claude-code-acp` is deprecated on npm ("renamed to
@agentclientprotocol/claude-agent-acp. Please migrate to continue receiving
updates.") and frozen at 0.16.2. The successor is
`@agentclientprotocol/claude-agent-acp` (github.com/agentclientprotocol/
claude-agent-acp), which installs a different binary name:
`claude-agent-acp`. `DefaultAdapterCommand` now targets it.

Re-validated versions: adapter 0.58.1 (@agentclientprotocol/sdk 1.2.1,
@anthropic-ai/claude-agent-sdk 0.3.205), same acp-go-sdk v0.13.5 and node
v22.19.0, same probe design (local pipes + dummy listener + live gateway
turns).

| Gate | 0.16.2 | 0.58.1 |
|---|---|---|
| initialize with acp-go-sdk v0.13.5 (protocolVersion 1) | pass | pass |
| Stdio purity (all runs) | pass | pass |
| ANTHROPIC_BASE_URL routing exclusivity | pass | pass |
| Resume hard gate (turn, SIGKILL, fresh process, session/resume, recall) | pass | pass (resume ~3.4s vs ~1.9s) |
| session/load fallback, history replayed as updates | pass | pass |
| session/cancel resolves stopReason=cancelled | pass (~8ms) | pass (~12ms) |
| Session storage `~/.claude/projects/<cwd-slug>/<id>.jsonl` | pass | pass |
| ANTHROPIC_MODEL honored in ACP mode | FAIL (finding 8) | PASS: requested model was `claude-haiku-4-5` with the env var set, `claude-opus-4-8` without |
| Usage over ACP | absent (finding 6) | PASS: `PromptResponse.usage` populated (input/output/cached tokens) plus `usage_update` session updates |

Both 0.16.2 gaps are fixed in 0.58.1: the runtime config `Model` knob works
through env injection (no `~/.claude/settings.json` workaround needed), and
per-turn usage lands in `TurnOutcome.Usage`. Capability changes: session
capabilities grew (`close`, `delete`, `additionalDirectories`), `authMethods`
is now empty (API-key env remains sufficient), and the same five session
modes are advertised (`default`, `acceptEdits`, `plan`, `dontAsk`,
`bypassPermissions`).

## Addendum: per-turn model switching over env (gate G1, 2026-07-10)

Per-message model selection rides the existing per-spawn `ANTHROPIC_MODEL`
injection, so the open question was whether the env pin also wins when the
turn *resumes* a prior session (every turn after the first). Probe design:
local adapter (latest `@agentclientprotocol/claude-agent-acp` via npm,
node v22.19.0, acp-go-sdk v0.13.5), a tee proxy in front of the live
gateway recording the `model` field of every request, and per-phase marker
strings in the prompt to attribute main-turn requests.

| Phase | Env | Session | Main-turn model requested |
|---|---|---|---|
| 1 | `ANTHROPIC_MODEL=claude-haiku-4-5` | session/new | `claude-haiku-4-5` |
| 2 | `ANTHROPIC_MODEL=claude-sonnet-4-5` | session/resume of phase 1 | `claude-sonnet-4-5` |
| 3 | unset | session/resume of phase 1 | `claude-sonnet-4-5-20250929` |

Verdict: **G1 passes**. The env pin wins over the resumed session's stored
model, so the env carrier supports per-chat and mid-chat switching with no
new protocol surface; the ACP `session/set_config_option` contingency stays
unused.

Known limitation (phase 3): with *no* env pin, a resumed session sticks to
the model it last used (in dated form) instead of reverting to the adapter
default. Consequence for model selection: after a user clears their pick
back to "Default" on an org with an *empty* admin model pin, subsequent
turns keep using the previously picked model until a new pick or an admin
pin injects `ANTHROPIC_MODEL` again. With a non-empty admin pin the pin is
injected on every unselected turn, so "Default" behaves as expected.

