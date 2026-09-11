# Chat actor dogfood harness

This harness emulates a first-party Slack bot that drives the Coder Agents (chats) API on behalf of several users in one chat. It checks that every turn is attributed to the user who posted it (the actor), not the chat owner: RBAC gates, workspace access, per-user MCP tokens, forwarded identity headers, gateway API keys, and audit rows.

It is a throwaway prototype harness for the `ethan/chat-actor-prototype` branch. It is not run in CI.

## Layout

| Path                | Purpose                                                                                               |
|---------------------|-------------------------------------------------------------------------------------------------------|
| `run.sh`            | One-command rerun: dev server, MCP server, `botemu setup`, `botemu tokens`, `botemu run`.             |
| `botemu/`           | Go program with subcommands `setup`, `tokens`, `run`. Uses `codersdk` and the dev Postgres directly.  |
| `mcpserver/`        | Go streamable HTTP MCP server named `whoami`. Echoes the forwarded `X-Coder-*` headers and a token label. |
| `run_mcpserver.sh`  | Builds and starts `mcpserver` with `TOKEN_LABELS` read from `state.json`.                              |
| `state.json`        | Ids and secrets shared between subcommands (mode 0600, git-ignored).                                   |
| `results.md`        | Verdict table and evidence for the last `botemu run` (git-ignored).                                    |
| `logs/`             | `develop.log`, `mcpserver.log`, `setup.log`, `tokens.log`, `run.log`, `whoami_calls.jsonl` (git-ignored). |

All commands run from the repository root. `botemu` reads the dev admin session from `.coderv2/session` and the dev Postgres port and password from `.coderv2/postgres/`.

## Prerequisites

- Docker, for the `docker` starter template. The harness creates the workspace `alice/alice-ws` and, in S9 and S9B, `bob/bob-from-chat`.
- `go`, `jq`, `curl`, and the normal `./scripts/develop.sh` toolchain.
- Ports `3000` (dev API), `8080` (dev web UI), and `3999` (MCP server) free on `127.0.0.1`.
- Environment variables:

| Variable                  | Required | Meaning                                                                                                  |
|---------------------------|----------|----------------------------------------------------------------------------------------------------------|
| `OPENAI_API_KEY`          | yes      | API key stored on the `openai` AI provider. Never printed. `run.sh` fails fast when it is unset.          |
| `OPENAI_BASE_URL`         | no       | OpenAI-compatible base URL. `run.sh` defaults it to `https://dogfood.cdr.dev/api/v2/ai-gateway/openai/v1` (Coder AI Gateway). |
| `CODER_DEV_SKIP_START`    | no       | Set to `1` to reuse a dev server already running on `127.0.0.1:3000` instead of starting one.            |
| `CODER_DEV_READY_TIMEOUT` | no       | Seconds to wait for the dev server banner and `/healthz`. Default `900`.                                  |

The model is `gpt-4.1-mini` (`botemu setup -model` overrides it). The dev server must run with `--experiments=oauth2` (OAuth2 provider), `--mcp-allowed-private-cidrs=127.0.0.0/8` (loopback MCP URL), and `--verbose` (S5 and S8 grep `actor_id=` debug lines from `logs/develop.log`).

## Run everything

```sh
OPENAI_API_KEY=... ./scripts/chatactor-dogfood/run.sh
```

`run.sh` does the following and stops what it started on exit:

1. Starts `CODER_DEV_STARTER_TEMPLATE=docker ./scripts/develop.sh -- --experiments=oauth2 --mcp-allowed-private-cidrs=127.0.0.0/8 --verbose` with output in `logs/develop.log`, then waits for `Coder is now running in development mode` and `GET /healthz`.
2. Builds `botemu` into `build/chatactor-dogfood/` and runs `botemu setup`: users `alice`, `bob`, `carol` (password `SomeSecurePassword!`), AI provider `openai` and model `gpt-4.1-mini`, MCP config `whoami` (`auth_type=oauth2`, `force_on`, `forward_coder_headers=true`, deny list `noop`), MCP bearer tokens for alice and bob seeded by SQL into `mcp_server_user_tokens`, OAuth2 apps `slack-bot` and `slack-bot-noshare`, and workspace `alice/alice-ws` from the `docker` template.
3. Starts `run_mcpserver.sh` on `127.0.0.1:3999` and waits for its `/healthz`.
4. Runs `botemu tokens`: PKCE authorization code flow for alice, bob, carol against `slack-bot`, plus one alice token against `slack-bot-noshare` without `chat:share`.
5. Runs `botemu run` with any arguments you passed and writes `results.md`.

When a dev server is already up, keep it running with output teed to `scripts/chatactor-dogfood/logs/develop.log`, then run `CODER_DEV_SKIP_START=1 ./scripts/chatactor-dogfood/run.sh`. Without that log file the S5 and S8 server-log checks fail.

`botemu setup` is safe to rerun against the dev database from an earlier session. It replaces the stored `openai` provider key when none of the stored keys masks to the current `OPENAI_API_KEY` (the AI Gateway key rotates between sessions; a stale key fails every turn with "Authentication with OpenAI failed"). It also starts `alice/alice-ws` again when the latest build is `running` but every agent is `disconnected` or `timeout`, which happens when the workspace container from the earlier session is gone.

## Scenarios

`botemu run` executes S0 to S8 by default. `-s9` adds S9. S4A and S9B run only when named with `-only`.

| ID  | Name                                     | Asserts                                                                                                                                                                                                                                            |
|-----|------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| S0  | Discovery                                | `GET /.well-known/oauth-authorization-server` lists `chat:read`, `chat:create`, `chat:use`, `chat:update`, `chat:delete`, `chat:share`, `chat:*`, and `workspace:share` in `scopes_supported`.                                                       |
| S1  | Token identity                           | For alice, bob, carol: `GET /api/v2/users/me` with the OAuth2 token as `Authorization: Bearer` and as `Coder-Session-Token` returns 200 with that user's id and username.                                                                         |
| S2  | Thread start                             | Alice's token creates a chat bound to `alice/alice-ws`; owner is alice, the `force_on` MCP config is attached, the first user message has `created_by` alice, the turn ends `waiting` with "pong", and `agent_id` resolves to the workspace agent. |
| S3  | Join ordering and sharing                | Bob posting before any grant is denied (403 or 404); alice's no-share token gets 403 from `PATCH /chats/{id}/acl`; alice's full token sets bob=`use` and carol=`read`, and `GET /chats/{id}/acl` round-trips both roles.                          |
| S4A | MCP whoami smoke (alice only)            | Same checks as S4 for alice only. Not in the default set.                                                                                                                                                                                          |
| S4  | Per-user MCP identity                    | Bob, then alice, ask for `whoami`. The tool result has `owner_id` alice, `actor_id` and `token_label` equal to the poster (the per-user MCP token follows the actor), the chat id, and the `whoami` `mcp_server_config_id`.                        |
| S5  | Workspace denial                         | Alice removes bob from the workspace ACL (a rerun may have left an S6 grant), then bob asks `execute hostname`; the tool result is an error containing `workspace access denied: user bob does not have access to workspace alice/alice-ws`, the assistant relays it, and `logs/develop.log` has `actor_id=<bob>` lines. |
| S6  | Share and retry                          | Alice shares `alice-ws` with bob as `use`; bob's `execute hostname` succeeds and the output contains the workspace name.                                                                                                                            |
| S7  | Handler gates                            | Bob (use) is denied archive and ACL updates; bob can interrupt a running turn; carol (read) is denied posting (403 or 404); the admin session is denied posting with 404 (admins hold `update`, not `use`); bob and carol can read the chat and messages. |
| S8  | Attribution                              | `api_keys` has exactly one `chatd_%_session_token` key each for alice and bob and none for carol or admin; `logs/develop.log` has `actor_id=` lines for alice and bob.                                                                             |
| S9  | Create workspace from chat (optional)    | Inside alice's bound chat, bob asks `create_workspace bob-from-chat`; the workspace exists owned by bob, the chat rebinds to it, bob's `execute` succeeds there, and alice's `execute` is denied.                                                    |
| S9B | Use sharer creates workspace (unbound)   | A leftover `bob/bob-from-chat` is deleted first. Alice creates an unbound chat and grants bob `use`; bob creates `bob-from-chat`. Owner and build initiator are bob, the chat rebinds while alice stays owner, all `audit_logs` rows are bob's, bob's `execute` and `stop_workspace` succeed, alice's are denied. |

### Rerun a single scenario

```sh
CODER_DEV_SKIP_START=1 ./scripts/chatactor-dogfood/run.sh -only S7
```

`run.sh` passes its arguments to `botemu run`. `-only` takes a comma-separated list (`-only S3,S7`). Every rerun of `run.sh` also reruns `setup` and `tokens`; both are idempotent. To skip them, call the binary directly:

```sh
go run ./scripts/chatactor-dogfood/botemu run -only S7
```

Scenarios after S2 reuse the chat stored as `chat_id` in `state.json`. Pass `-new-chat=false` to keep that chat when S2 or S9B are rerun; the default creates a fresh chat.

### Re-mint tokens when scopes change

`botemu tokens` requests the scope set in `botScopes` (`botemu/main.go`) and stores the tokens in `state.json`. Override it without editing code:

```sh
go run ./scripts/chatactor-dogfood/botemu tokens -scopes "chat:create chat:read chat:use chat:update chat:share workspace:read workspace:share user:read_personal user:read workspace:ssh chat_model_config:read"
```

The no-share token always uses `noShareScopes`. Known results by scope set:

- Without `user:read` and `workspace:ssh`: S1 failed, `GET /api/v2/users/me` returned 404.
- Without `chat_model_config:read`: S2 failed, `POST /api/v2/chats` returned 400 "No chat model is available in this organization."
- The full set above: S0 to S8 pass.

### Bot scope set

Posting messages, submitting tool results, and interrupting a chat require the `chat:use` scope and the `use` action on the chat. Chat settings, the queue, clear, and retitle require `chat:update`. A bot that only posts on behalf of users needs this minimum set:

```text
chat:create chat:read chat:use chat:share workspace:read workspace:share workspace:ssh user:read user:read_personal chat_model_config:read
```

`botScopes` keeps `chat:update` on top of that set on purpose. With the scope present, the S7 `PATCH /chats/{id}` archive denial for bob comes from the chat ACL (bob holds `use`, not `update`), not from a missing scope.

### Results and logs

- `scripts/chatactor-dogfood/results.md`: verdict table plus the evidence lines of each scenario from the last `botemu run`. Every run overwrites it.
- `scripts/chatactor-dogfood/logs/`: `develop.log` (dev server, verbose), `mcpserver.log`, `whoami_calls.jsonl` (one JSON line per `whoami` call), `setup.log`, `tokens.log`, `run.log`.
- `scripts/chatactor-dogfood/state.json`: ids and secrets. Delete it to start from scratch against a reset database. Deleting it against an existing database recreates the OAuth2 apps and mints new MCP tokens.

## Last known results

Branch `ethan/chat-actor-prototype`, 2026-09-11, model `gpt-4.1-mini` through the Coder AI Gateway, against the dev database of the 2026-09-10 run. S0 to S8 ran with one `run.sh` invocation on `7688b2eedc`. S9B ran with `run.sh -only S9B` on `89950b0379`, which contains the fix that S9B found (see the S9B note). Rerun `run.sh` to refresh the table.

| ID  | Verdict | Notes                                                                                                                                                                                                                                                                              |
|-----|---------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| S0  | PASS    | `scopes_supported` includes `chat:use`.                                                                                                                                                                                                                                            |
| S1  | PASS    |                                                                                                                                                                                                                                                                                    |
| S2  | PASS    | The first rerun failed every turn with "Authentication with OpenAI failed" because the stored provider key was the previous session's AI Gateway key. `setup` now replaces a stale key.                                                                                             |
| S3  | PASS    | bob=`use`, carol=`read` round-trip.                                                                                                                                                                                                                                                |
| S4  | PASS    |                                                                                                                                                                                                                                                                                    |
| S5  | PASS    | The first rerun failed because the 2026-09-10 S6 grant was still on the workspace ACL and bob's `execute` succeeded. S5 now revokes bob before posting.                                                                                                                             |
| S6  | PASS    |                                                                                                                                                                                                                                                                                    |
| S7  | PASS    | Admin post returns 404. Interrupt ran while the chat was running.                                                                                                                                                                                                                  |
| S8  | PASS    |                                                                                                                                                                                                                                                                                    |
| S9  | FAIL    | 2026-09-10 result, not rerun. In the bound chat the model called `create_workspace`, which returned `already_exists` for `alice-ws`. Superseded by S9B.                                                                                                                              |
| S9B | PASS    | The first rerun failed step 4: bob's `create_workspace` created the workspace but `UpdateChatWorkspaceBinding` returned `rbac: forbidden` because dbauthz required `update` and bob holds `use`. Fixed in `9b19182b8a` (the binding accepts `use` or `update`); the rerun passed fully. |

The 2026-09-10 run on `9a50b4fde1` (before the chat `use` action) passed S0 to S8 and S9B with bob as `write`.
