# Configure chat lifecycle hooks

This reference is for Coder deployment administrators who need to apply an external policy service to the agent loop.
It covers deployment configuration, the consumer contract, failure behavior, rollout, and dispatch auditing.

Chat lifecycle hooks send events from the agent loop to 1 deployment-wide webhook endpoint.
The configured consumer can observe all 7 lifecycle events, add model or user context, restrict tools, replace mutable input, deny selected actions, or end a chat.

> [!IMPORTANT]
> A consumer can block agent activity across the deployment.
> Start with an observe-only consumer and test failure recovery before enforcing policy.

## Configure the deployment

Set the following deployment options on `coder server`.

| Environment variable      | CLI flag              | Default | Requirement                                                                          |
|---------------------------|-----------------------|---------|--------------------------------------------------------------------------------------|
| `CODER_CHAT_HOOK_URL`     | `--chat-hook-url`     | Empty   | Use an `https` URL. Hooks are inactive when this value is empty.                     |
| `CODER_CHAT_HOOK_SECRET`  | `--chat-hook-secret`  | Empty   | Required when the hook URL is set. Coder uses this shared secret to sign HS256 JWTs. |
| `CODER_CHAT_HOOK_TIMEOUT` | `--chat-hook-timeout` | `1.5s`  | Must be greater than `0` and no more than `5s`. The timeout applies to each request. |
| `CODER_CHAT_HOOK_ENABLED` | `--chat-hook-enabled` | `true`  | Set to `false` to stop dispatching without removing the URL or secret.               |

Treat `CODER_CHAT_HOOK_ENABLED=false` as the break-glass control.
Changing deployment options requires the normal `coder server` configuration rollout for your installation.

Use a dedicated secret and rotate it through your existing secret-management process.
Coder requires the configured URL to use HTTPS.
A TLS terminator can forward the request to a consumer over plain HTTP on a trusted local network.
It must preserve the original `Host` header and set `X-Forwarded-Proto: https` for the SDK handler's audience check.

## Handle lifecycle events

Coder sends an HTTP `POST` request for each event.
The JSON body contains `type`, `meta`, and event-specific `data`.
The `meta` object includes `dispatch_id`, `schema_version`, `chat_id`, `owner_id`, and optional workspace and turn IDs.
The current `schema_version` is `1`.

| Event                | When Coder sends it                        | Decision-relevant data                                                 |
|----------------------|--------------------------------------------|------------------------------------------------------------------------|
| `session_start`      | A chat session starts, resumes, or clears  | `source` (`startup`, `resume`, or `clear`)                             |
| `user_prompt_submit` | A user submits a prompt                    | `prompt` and `parts`                                                   |
| `pre_tool_use`       | Before a non-provider-executed tool runs   | `tool_use_id`, `tool_name`, and `tool_input`                           |
| `post_tool_use`      | After a non-provider-executed tool returns | `tool_use_id`, `tool_name`, and either `tool_response` or `tool_error` |
| `pre_compact`        | Before Coder compacts chat context         | No event-specific fields                                               |
| `post_compact`       | After Coder compacts chat context          | No event-specific fields                                               |
| `stop`               | The model stops a turn                     | No event-specific fields                                               |

Provider-executed tools don't produce `pre_tool_use` or `post_tool_use` events because the provider executes them outside Coder's tool runtime.

For `user_prompt_submit`, `prompt` concatenates the text parts of the message, and `parts` carries the full structured message exactly as Coder stores it and sends it to the model, including non-text parts such as file references.
A consumer that gates prompt content must inspect `parts`.

### Verify each request

Coder sends the JWT in the `Authorization: Bearer <token>` header.
A consumer must apply all of the following checks before it uses the body:

- Accept only the `HS256` algorithm and verify the signature with `CODER_CHAT_HOOK_SECRET`.
- Check that `iss` is the Coder deployment ID associated with the secret.
- Check that `aud` exactly matches `CODER_CHAT_HOOK_URL`.
- Check `nbf` and reject expired tokens using `exp`.
- Check that `jti` equals the request `meta.dispatch_id`.
- Check that the JWT event `type` equals the body event `type`.
- Compute SHA-256 over the exact request body bytes and compare it with `body_sha256`.
- Check that the chat ID in `sub` matches `meta.chat_id`.

The Go consumer SDK in `codersdk/agenthooks` implements the wire types, JWT verification, body binding, audience checks, and event routing.
Use `agenthooks.NewHTTPHandler` to build an `http.Handler` from callbacks for the events your consumer handles.
Use a secret dedicated to the expected deployment, or add an issuer check when you implement JWT verification directly.

### Return a response

Return any `2xx` status with an empty body for a no-op response.
An empty JSON object has the same effect.
If the response has a body, return a JSON object with these optional fields.

| Field           | Effect                                                                                                                                                                                                                                                                |
|-----------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `permission`    | Allows or denies mutable input for `user_prompt_submit` and `pre_tool_use` only.                                                                                                                                                                                      |
| `model_context` | Adds text visible to the model. The value is limited to 16&nbsp;KiB.                                                                                                                                                                                                  |
| `user_message`  | Adds a system message visible to the user.                                                                                                                                                                                                                            |
| `allowed_tools` | Replaces the chat's hook tool policy. Omit the field to preserve the policy, return `[]` to restrict all tools, or return names to allow only those tools. The policy applies from the next generation step onward, and a call to an excluded tool fails as inactive. |
| `end_chat`      | Ends the chat when `true`.                                                                                                                                                                                                                                            |

The `permission.decision` value supports `allow` and `deny`.
The `ask` value isn't supported and causes the dispatch to fail closed.

Permission rules depend on the event:

- For `user_prompt_submit`, `allow` requires `input_override` in the exact form `{"prompt":"replacement text"}`.
  Coder stores and sends the replacement prompt instead of the original prompt.
- For `pre_tool_use`, `allow` requires `input_override` containing the replacement tool input.
  Coder persists the replacement with the tool call and executes the tool with it.
- For either event, `deny` blocks the input.
  A denied prompt isn't persisted, and a denied tool call becomes a synthetic error result so the model can choose another action.
- For all other events, omit `permission`.

## Plan failure recovery

Lifecycle hooks are fail closed.
Coder treats a timeout, connection failure, non-`2xx` response, malformed response, or unsupported response field combination as a hook dispatch failure.
An in-progress chat enters the error state and records the dispatch ID in its error details.
If the first `user_prompt_submit` dispatch fails during chat creation, Coder rejects the request and doesn't create the chat.
If `post_tool_use` fails for a client-submitted tool result, Coder rejects the submission without committing the results, and the client can resubmit them after the consumer recovers.
If `post_tool_use` fails for a tool that Coder already executed, Coder commits the tool result first so the transcript reflects the completed side effect, then moves the chat to the error state.
An `end_chat` instruction that Coder already accepted from a successful dispatch in the same step takes precedence over the error state: if `post_tool_use` or `post_compact` fails after an accepted `end_chat`, Coder still ends the chat and records the failed dispatch.

After the consumer is healthy, send another message to an existing errored chat to resume it.
Coder emits `session_start` with `source` set to `resume` when the agent loop starts again.
If the consumer continues blocking chat activity, set `CODER_CHAT_HOOK_ENABLED=false` and roll out the Coder deployment configuration before users retry.

## Roll out enforcement in stages

Use the following rollout sequence:

1. Deploy a consumer that verifies every request, logs the event and identifiers, and always returns a `2xx` status with an empty response.
2. Configure the hook URL, secret, and timeout on a test deployment.
3. Exercise normal chats, tool calls, compaction, consumer timeouts, and consumer restarts.
4. Review `chat_hook_dispatches` for event coverage and unexpected failures.
5. Add policy responses for a narrow event or tool set.
6. Expand enforcement after the dispatch audit shows the expected decisions and failure rate.

Keep the break-glass procedure available throughout the rollout.

## Start from the reference consumer

The reference consumer at `scripts/agenthooks-server` uses `agenthooks.NewHTTPHandler` and logs 1 JSON object for each event.
Log-only mode returns an empty response for every verified event.
With log-only mode disabled, the optional example flags can deny tool names by regular expression or replace matching prompt text before the agent loop uses it.
Coder retains the original prompt in the dispatch audit row.

Run the consumer from a Coder source checkout:

```sh
CODER_AGENTHOOKS_SECRET='<shared-secret>' \
  go run ./scripts/agenthooks-server \
  --listen 127.0.0.1:8081 \
  --log-only=true
```

The reference server accepts optional TLS certificate and key paths.
For local testing with plain HTTP, place an HTTPS reverse proxy in front of it because `CODER_CHAT_HOOK_URL` accepts only HTTPS URLs.
Run `go run ./scripts/agenthooks-server --help` for all flags and environment variable names.

## Audit dispatches

Coder records every attempted dispatch in the `chat_hook_dispatches` database table.
Each row includes the event, chat and turn identifiers, tool-use ID when present, timestamps, HTTP status, result class, permission decision, input override, response context, and error details.
Use the `dispatch_id` from the request or chat error as the row ID when correlating consumer logs with Coder state.

The table can contain prompts, tool input, response context, and user messages.
Apply the same access controls and database protection that you use for other sensitive chat data.
The `dbpurge` service removes dispatch rows after 90 days to bound table growth.
