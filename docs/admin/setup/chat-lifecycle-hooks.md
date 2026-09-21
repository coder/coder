# Configure chat lifecycle hooks

> [!NOTE]
> Chat lifecycle hooks are an experimental feature.
> The feature requires the `agent-lifecycle-hooks` experiment, and the consumer contract (including the request schema and JWT claims) may change or be removed in any release without a compatibility guarantee.

This guide is for Coder deployment administrators who need to apply an external policy service to the agent loop.
Work through it to configure the deployment, handle events in a consumer, recover from dispatch failures, and roll out enforcement.

Chat lifecycle hooks send events from the agent loop to 1 deployment-wide webhook endpoint.
The configured consumer can observe all 7 lifecycle events, add model or user context, replace mutable input, or deny selected actions.
Coder keeps no record of dispatched events or consumer decisions: any policy state, audit trail, or decision history lives in the consumer.

> [!IMPORTANT]
> A consumer can block agent activity across the deployment.
> Start with an observe-only consumer and test failure recovery before enforcing policy.

## Configure the deployment

Enable the experiment first:

```env
CODER_EXPERIMENTS=agent-lifecycle-hooks
```

Without the experiment, an enabled hook configuration is inactive: `coder server` logs a warning at startup and dispatches no hook events.
Enabled hook settings are still validated at startup when a hook URL is set. Leaving the URL unset, or setting `CODER_CHAT_HOOK_ENABLED=false`, makes the hook settings inert and skips their validation.
The experiment list is read at startup, so enabling or disabling it requires a `coder server` restart.

Set the following deployment options on `coder server`.

| Environment variable             | CLI flag                     | Default | Requirement                                                                                                                                                      |
|----------------------------------|------------------------------|---------|------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `CODER_CHAT_HOOK_URL`            | `--chat-hook-url`            | Empty   | Use an `https` URL, or an `http` URL with `CODER_CHAT_HOOK_ALLOW_INSECURE`. Hooks are inactive when this value is empty.                                         |
| `CODER_CHAT_HOOK_SECRET`         | `--chat-hook-secret`         | Empty   | Required when the hook URL is set, at least 32 bytes of cryptographically random data. Coder uses this shared secret to sign HS256 JWTs.                         |
| `CODER_CHAT_HOOK_TIMEOUT`        | `--chat-hook-timeout`        | `1.5s`  | Must be greater than `0` and no more than `5s`. The timeout applies to each request.                                                                             |
| `CODER_CHAT_HOOK_ENABLED`        | `--chat-hook-enabled`        | `true`  | Set to `false` to stop dispatching without removing the URL or secret.                                                                                           |
| `CODER_CHAT_HOOK_ALLOW_INSECURE` | `--chat-hook-allow-insecure` | `false` | Set to `true` to allow a plain `http` hook URL. Plain HTTP lets an attacker on the network forge hook responses, so only enable it on a network you fully trust. |

Treat `CODER_CHAT_HOOK_ENABLED=false` as the break-glass control.
Changing deployment options requires the normal `coder server` configuration rollout for your installation.

Use a dedicated secret and rotate it through your existing secret-management process.
Rotation is a hard cutover: Coder signs with exactly one secret, so dispatches fail until the consumer accepts the new value.
Rotate during a maintenance window, or temporarily set `CODER_CHAT_HOOK_ENABLED=false` for the cutover if blocked chats are worse than unreviewed ones for your deployment.
Coder requires the configured URL to use HTTPS unless `CODER_CHAT_HOOK_ALLOW_INSECURE` is set.
Plain HTTP removes more than transport privacy: hook responses are what allow or deny tool calls and can rewrite prompts and tool inputs, so anyone on the network path can forge them. Coder logs a warning at startup when hooks run over plain HTTP.
A TLS terminator can forward the request to a consumer over plain HTTP on a trusted local network.
Configure the consumer with the same `CODER_CHAT_HOOK_URL` value, because that URL is the audience Coder signs into every dispatch.
The consumer compares the `aud` claim against its configured audience and rejects a mismatch.
It derives nothing from the request URL, the `Host` header, or forwarding headers, so the number of proxy hops in front of it doesn't affect the check.

## Handle lifecycle events

Coder sends an HTTP `POST` request for each event.
The JSON body contains `type`, `meta`, and event-specific `data`.
The `meta` object includes `dispatch_id`, `schema_version`, `chat_id`, `owner_id`, and optional workspace and turn IDs.
Events from subagent chats also carry `parent_chat_id` and `root_chat_id` so a consumer can correlate a subagent subtree with the user-facing conversation and apply the parent's policy context.
The current `schema_version` is `1`.

Handle the events your policy needs, using the data Coder sends with each one:

| Event                | When Coder sends it                                                 | Decision-relevant data                                                 |
|----------------------|---------------------------------------------------------------------|------------------------------------------------------------------------|
| `session_start`      | A chat session starts, resumes, or clears                           | `source` (`startup`, `resume`, or `clear`)                             |
| `user_prompt_submit` | A user submits a prompt, or `spawn_agent` submits a subagent prompt | `prompt` and `parts`                                                   |
| `pre_tool_use`       | Before a non-provider-executed tool runs                            | `tool_use_id`, `tool_name`, and `tool_input`                           |
| `post_tool_use`      | After a non-provider-executed tool returns                          | `tool_use_id`, `tool_name`, and either `tool_response` or `tool_error` |
| `pre_compact`        | Before Coder compacts chat context                                  | No event-specific fields                                               |
| `post_compact`       | After Coder compacts chat context                                   | No event-specific fields                                               |
| `stop`               | The model stops a turn                                              | No event-specific fields                                               |

Provider-executed tools don't produce `pre_tool_use` or `post_tool_use` events because the provider executes them outside Coder's tool runtime.

For `pre_tool_use`, `tool_input` carries the model's JSON bytes with key spelling and order preserved, so a policy can read them exactly as the model wrote them.
Coder's built-in tools decode that JSON with Go, which matches property names case-insensitively and keeps the last match, so `{"path":"/allowed","PATH":"/secret"}` could make a policy approve `/allowed` while the tool opens `/secret`.
Coder rejects a built-in tool call whose input repeats a key or spells a schema property with different capitalization, before dispatching `pre_tool_use`.
This check doesn't cover dynamic and MCP tools, because the client and the workspace agent execute those calls rather than coderd.
A policy that gates them must validate their input itself.
The chat `edit_files` tool reads `old_text` and `new_text` and, for rollout compatibility, also accepts the deprecated `search` and `replace` keys when the new fields are empty. A policy that gates edit content should inspect `old_text` and `new_text`, and `search`/`replace` while the compatibility window lasts.

For `user_prompt_submit`, `prompt` concatenates the original submitted text parts, and `parts` carries the original structured message, including non-text parts such as file references.
These values are captured before the consumer's override or injected context changes the stored prompt.
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
- Check that `sub` has the form `coder:chat:<chat ID>` and that its chat ID matches `meta.chat_id`.

The Go consumer SDK in `codersdk/x/agenthooks` implements the wire types, JWT verification, body binding, audience checks, and event routing.
Use `agenthooks.NewHTTPHandler` to build an `http.Handler` from callbacks for the events your consumer handles.
Pass the deployment's `CODER_CHAT_HOOK_URL` as the expected audience, because a handler built without one rejects every request.
Pass `agenthooks.WithExpectedIssuer` with the deployment ID associated with the secret to enforce the `iss` check.
Without it, `NewHTTPHandler` accepts any non-empty `iss` signed with the shared secret, so use a secret dedicated to one deployment or always set the expected issuer.

### Return a response

Return any `2xx` status with an empty body for a no-op response.
An empty JSON object has the same effect.
If the response has a body, return a JSON object with these optional fields.
Coder rejects a response body with unknown fields, duplicate JSON keys, or trailing data as malformed, so the dispatch fails closed instead of misreading the decision.

| Field           | Effect                                                                                                                                                                         |
|-----------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `permission`    | Allows or denies mutable input for `user_prompt_submit` and `pre_tool_use` only.                                                                                               |
| `model_context` | Adds text visible to the model. The value is limited to 16&nbsp;KiB. The text is absent from the user-visible transcript, but users may infer its effects from model behavior. |
| `user_message`  | Adds a message visible to the user.                                                                                                                                            |

The `permission.decision` value supports `allow` and `deny`.
Any other value causes the dispatch to fail closed.

Permission rules depend on the event:

- For `user_prompt_submit`, `allow` requires `input_override` in the exact form `{"prompt":"replacement text"}`.
  Coder stores and sends the replacement prompt instead of the original prompt.
  The override replaces only submitted text, matching the concatenated `prompt` field the consumer receives.
  Attachments and file references remain in `parts`, so consumers that must block them should inspect `parts` and return `deny`.
- For `pre_tool_use`, `allow` requires `input_override` containing the replacement tool input.
  Coder persists the replacement with the tool call and executes the tool with it.
  An override for a built-in tool must not repeat a key or vary the capitalization of a schema property; an ambiguous override fails the dispatch closed because the model can't correct it.
  The stored call is marked as rewritten, and the chat shows a "Modified by policy" badge.
  The marker is client-facing, so return `model_context` if the model also needs an explanation of the rewrite.
- For either event, `deny` blocks the input and must not include `input_override`.
  A denied prompt isn't persisted: Coder rejects the submission and surfaces any returned `user_message` in the rejection, ignoring `model_context`.
  A denied tool call becomes a synthetic error result, and any returned `model_context` reaches the model separately, so the model can choose another action.
- For all other events, omit `permission`.

For `user_prompt_submit`, `model_context` and `user_message` are stored as typed parts of the prompt message itself: the model-context part goes to the model but never to clients, and the user-message part is shown to the user attached to the prompt but never sent to the model.
For a denied `pre_tool_use`, the synthetic denied tool result stays visible to both audiences, while `model_context` becomes a model-only transcript message so it never reaches clients.
Other hook effects become ordinary transcript messages with audience-specific visibility, except that a `pre_compact` `model_context` guides the compaction summary instead of entering the transcript.
Coder dispatches `user_prompt_submit` exactly once per submission, when the prompt is admitted (sent, queued, edited, or used to create a chat or subagent), and applies the response effects to the final stored prompt content.

### When tool calls are admitted

Coder dispatches `pre_tool_use` once per tool call, after the model finishes proposing it and before the assistant message is stored. The stored message therefore carries the input the tool actually runs with, and stored message content is never rewritten afterwards.

Two consequences follow:

- Clients stream the model's proposed tool input while the dispatch is in flight, then converge on the stored input once the message is committed.
  A rewritten call keeps displaying the original input until the whole batch is admitted, and the row spinner remains after the live **Thinking** indicator clears.
  Coder dispatches a batch sequentially, so that window is bounded by `CODER_CHAT_HOOK_TIMEOUT` multiplied by the number of tool calls in the step, not by a single timeout.
- A tool call that is already in chat history was admitted before it was stored, so Coder executes it with the stored input instead of dispatching a second decision. If a consumer's policy changes between those two points, the change applies to later calls, not to calls already admitted. A call stored before hooks were configured is likewise not admitted retroactively, the same way an earlier prompt isn't.

The per-chat debug endpoint records what the model proposed, including tool input that a consumer replaced. It reports provider behavior and is not part of the chat transcript.

## Plan failure recovery

Lifecycle hooks are fail closed.
Coder treats a timeout, connection failure, non-`2xx` response, malformed response, or unsupported response field combination as a hook dispatch failure.
A failure during generation moves the chat to the error state and records the dispatch ID in its error details.
A failed prompt dispatch for an existing idle chat can also move that chat to the error state even though the API request is rejected.
If the first `user_prompt_submit` dispatch fails during chat creation, Coder rejects the request and doesn't create the chat.
If `post_tool_use` fails for a client-submitted tool result, Coder rejects the submission without committing the results, and the client can resubmit them after the consumer recovers.
If a hook dispatch fails after Coder has already executed one or more tools in a batch, Coder commits the tool results first so the transcript reflects the completed side effects, then moves the chat to the error state.
This covers a failed `post_tool_use` for an executed tool and a failed `user_prompt_submit` for a subagent spawn, where the spawn is refused but the tools that ran alongside it keep their results.

Dispatch precedes persistence, so a delivered event doesn't guarantee that the operation commits.
Coder checks admission before dispatching, but concurrent requests can still fail admission afterward, for example two sends racing for the last queue slot or duplicate submissions of the same tool results.
The consumer then observes an event for a request that Coder rejects, and the rejected request doesn't persist a prompt or tool result.
Treat events as attempt notifications rather than proof of a committed operation, and key idempotent tool-event processing on `tool_use_id`.

Each `coderd` replica runs at most 256 dispatches at once and waits up to 250&nbsp;ms for a free slot; a dispatch that waits out that limit fails as over capacity.
The limit is per replica rather than deployment-wide, so size the consumer for 256 concurrent requests per replica.
Slow consumer responses hold slots for longer, so a slow consumer turns a burst of chat activity into over-capacity failures.
Prompt admission (creating, sending, or editing a message) can hold at most 192 of a replica's slots, so at least 64 stay reachable only by dispatches for work a chat already admitted.
That bound stops a burst of new submissions from consuming every slot, but it doesn't make the remaining slots sufficient: a saturated dispatcher can still fail a dispatch for a running chat and leave that chat in the error state.
Watch `coderd_chatd_hook_dispatches_total{result="over_capacity"}` to see whether the consumer's latency is turning normal traffic into rejections.

Delivery is best-effort and can duplicate.
Coder never queues a failed dispatch for redelivery, so plan for duplicates without assuming every event arrives.
Coder retries one connection failure per dispatch with the same JWT, so use `dispatch_id` to recognize a repeated HTTP attempt and return the same response.
Coder also re-dispatches the same logical event with a new `dispatch_id` whenever an operation runs again, for example when a chat recovers after a crash and retries a pending tool call, or when a user retries a turn that failed before committing.
Every tool call that reaches execution is validated through a fresh `pre_tool_use` dispatch; Coder never reuses an earlier decision on the consumer's behalf.
Calls that Coder rejects before execution, such as input that isn't valid JSON, ambiguous input, or a batch that mixes an exclusive tool with other calls, produce an error result for the model without a dispatch.

Use event-specific identifiers for logical duplicates:

| Events                                | Deduplication guidance                                                                                                                     |
|---------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------|
| `pre_tool_use`, `post_tool_use`       | Key by `chat_id`, event type, and `tool_use_id`.                                                                                           |
| `session_start`                       | Response effects can repeat after runner or process replacement. Make them safe to apply more than once.                                   |
| `user_prompt_submit`                  | Dispatch occurs before a message ID exists. Avoid non-idempotent external effects because identical prompt content can be submitted twice. |
| `pre_compact`, `post_compact`, `stop` | Don't rely on `turn_id` surviving recovery. Make external effects safe to repeat.                                                          |

Rejecting duplicates breaks Coder's retries. Return the same decision whenever the payload identifies the same logical event.

After the consumer is healthy, send another message to an existing errored chat to resume it.
Coder emits `session_start` when the agent loop starts again, with `source` set to `resume` when the chat already contains an assistant reply and `startup` otherwise.
If the consumer continues blocking chat activity, set `CODER_CHAT_HOOK_ENABLED=false` and roll out the Coder deployment configuration before users retry.

## Roll out enforcement in stages

Use the following rollout sequence:

1. Deploy a consumer that verifies every request, logs the event and identifiers, and always returns a `2xx` status with an empty response.
2. Configure the hook URL, secret, and timeout on a test deployment.
3. Exercise normal chats, tool calls, compaction, consumer timeouts, and consumer restarts.
4. Review the consumer's own logs for event coverage and unexpected failures.
5. Add policy responses for a narrow event or tool set.
6. Expand enforcement after the consumer logs show the expected decisions and failure rate.

Keep the break-glass procedure available throughout the rollout.

## Start from the reference consumer

The reference consumer at `scripts/agenthooks-server` uses `agenthooks.NewHTTPHandler` and logs 1 JSON object for each event.
Log-only mode returns an empty response for every verified event.
With log-only mode disabled, the optional example flags can deny tool names by regular expression or replace matching prompt text before the agent loop uses it.
It also demonstrates consumer-owned state: it remembers `pre_tool_use` decisions in memory keyed by chat and tool-use ID, replays them for duplicate deliveries, and marks the duplicates in its log output.

Run the consumer from a Coder source checkout:

```sh
CODER_AGENTHOOKS_SECRET='<shared-secret>' \
  go run ./scripts/agenthooks-server \
  --listen 127.0.0.1:8081 \
  --audience 'https://hooks.example.com' \
  --log-only=true
```

The server confirms the listener and then stays in the foreground, printing 1 JSON object per event it receives:

```output
Agent hooks server listening on 127.0.0.1:8081 in log-only mode
```

The reference server accepts optional TLS certificate and key paths.
For local testing with plain HTTP, either set `CODER_CHAT_HOOK_ALLOW_INSECURE=true` and use the `http` URL directly, or place an HTTPS reverse proxy in front of the consumer and pass the proxy's URL as `--audience`.
Run `go run ./scripts/agenthooks-server --help` for all flags and environment variable names.

## Audit dispatches

Coder doesn't store dispatched events or consumer decisions.
The consumer's own logs are the audit trail: log the `dispatch_id`, the stable identifiers, and the returned decision for every event.
Use the `dispatch_id` recorded in a chat's error details to correlate a failed dispatch with the consumer's logs.
Consumer logs can contain prompts, tool input, response context, and user messages, so apply the same access controls that you use for other sensitive chat data.
