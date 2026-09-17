# Coder Agents ACP Support

## TODO

- describe how runtime accounting will work
- describe how the workspace agent is going to handle
    - [ ] delivering messages
	- [ ] keeping track of the chat history
	- [x] loading config options into the database
- describe how custom harnesses will be configured, both in the UI and in templates
- describe how we will handle switching harnesses. what's going to happen on the code level when a user submits a message with a new harness/workspace configured?


## Database changes

### `chats` table

New columns:

- `harness`: enum with 2 values: builtin and acp
- `acp_config_option_values`: nullable JSONB
- `template_config`: stores the template configuration supplied to "create chat".

### `agents_harnesses` new table

Columns:

- id
- slug
- display_name
- icon
- created_at
- updated_at

### `agents_harness_links` new table

Table name TBD. Columns:

- id
- agents_harness_slug
    - Not a foreign key. We deliberately don’t want to resolve the harness at the time of creating the agents_harness_link. This allows a user to configure the template to support ACP independently of the harness in the db. If we used a foreign key they’d have to first create the db-level harness, and only then configure the template.
- template_version_id
- startup_script (string)
- created_at
- updated_at

### `acp_config_options` new table

It will contain dynamically discovered ACP config options. Rows will be added to that table after a workspace agent starts an ACP adapter and queries it for available options.

Columns:

- id
- agents_harness_link_id
- workspace_agent_id
- config_options JSONB
- created_at
- updated_at

## Creating an ACP chat

To create an ACP chat, a user selects "change harness" in the "plus" picker on the New Chat page. Then they select a compute: either a workspace or a template. Available workspaces and templates for a given harness are listed using a new endpoint. That endpoint only returns workspaces created using a template version that references the selected harness. It also only returns templates whose active template version references the selected harness.

Users may also configure "config options" exposed by ACP adapters. They are dynamically discovered by the workspace agent when it starts an ACP adapter. If a user selects a workspace in the UI, we display the latest config options in the database for that workspace. If no options are associated with that workspace, we display the latest options for that workspace’s template version scoped to that user. If still no options, we display the latest options for that workspace’s template, scoped to that user. If none are available, we show a fallback message. If the user selected a template, we show the latest options for the active template version scoped to that user. If none are available, we show the latest for the entire template scoped to that user. If none are available, we show a fallback message. We’re careful never to show options discovered in another user’s workspace. If we did, a malicious user would be able to poison other users' options.

The existing create chat endpoint accepts the new parameters. We’ll modify the CreateChat state machine transition to support them.

## Submitting messages to ACP chats

In contrast to the builtin harness, the HTTP endpoint always submits user messages to the queue instead of the chat_messages table directly. Only the ACP runner adds rows to the chat_messages table. The goal is to create the guarantee that if a message is in the chat_messages table, then the harness running over ACP is aware of it.

## ACP runner

We’re going to implement a new runner type in chatd. The acquisition loop will make a decision which runner to spawn based on the chat type: the existing runner for the builtin harness and a new ACP runner for ACP chats.

The ACP runner will be responsible for starting and creating workspaces for chats, and dialing out to workspace agents running ACP adapters.

### Behavior based on chat status

The runner will spawn goroutines in a loop based on the current status of the chat. These goroutines will do the following things:

- W: exit the goroutine, request runner shutdown.
- R0/R1: Run the [ensure workspace procedure](#ensure-workspace-procedure). Dial out to harness. If the harness is idle and we're in R1, submit one queued message to the agent and perform a transition to dequeue the message - in that order. This guarantees at least once delivery. Run the [message sync procedure](#message-sync-procedure). Increment the chat's `generation_attempt`. Call `StreamMessageParts` on the [ACP Dialer](#acp-dialer) with the latest values for the arguments. Wait until the agent becomes idle and all of its output is saved in the message part buffer. Submit the message parts stored in the buffer to the database. If we're in R0, move to W.
- I0/I1:
    - If the chat's harness or workspace is different than
    - If there's no attached workspace or it's stopped: if we're in I0, move to W and exit. If we're in I1, move to R1.
    - Otherwise run the [message sync procedure](#message-sync-procedure). Increment the chat's `generation_attempt`. Call `StreamMessageParts` on the [ACP Dialer](#acp-dialer) with the latest values for the arguments. Dial out to harness requesting interrupt. Wait until the harness stops and all of its output is saved in the message part buffer. Save the messages to the db. If I0, move to W. If I1, move to R1.
- A0/A1: exit the goroutine, request runner shutdown. We will enforce that ACP chats cannot move into the A0/A1 states at the database level.
- E0/E1: exit the goroutine, request runner shutdown.
- XW/XE0/XE1: exit the goroutine, request runner shutdown.

When the chat status changes, a goroutine running for a different status has its context cancelled. The runner spawns a new goroutine once the previous one exits.

### Retries

If a goroutine exits with an unexpected error, it will be retried 3 times. Then the chat will be moved into the E0 or the E1 state depending on queue occupancy.

### Ensure workspace procedure

The ensure workspace procedure makes sure that there's a running workspace attached to the chat. In case there's no workspace yet, it's created using the template configuration specified on the chat row.

Before creating or starting a workspace, the procedure increments the chat's `generation_attempt`. Then it writes either a `start_workspace` or `create_workspace` tool call to the current episode in the message parts buffer (it does not write them to message history), and runs the implementation of the tool. This allows the frontend to show the rich build logs preview while the workspace is being created or started. Neither the tool call nor its result are ever written to the chat history. They are ephemeral and disappear when the message part buffer episode changes.

### Message sync procedure

The message sync procedure compares 2 chat histories: the one that ACP returns, and the one Coder has in the database.

The procedure works as follows:

1. Find the last common message between the histories by comparing ids using [ACP Dialer's](#acp-dialer) `GetMessageParts`. We should only consider the database history that's the suffix after the most recent [new session marker](#new-session-marker), or all history if there's no marker. If a marker exists and there's no common message after it, consider the first user-visible user message after the marker in the database to correspond to the first user message in the ACP session. This preserves the special [first message after switching to an ACP harness](#first-message-after-switching-to-an-acp-harness).
2. Delete the chat history from the database after the last common message. Only consider messages after the most recent [new session marker](#new-session-marker), or all history if there's no marker. We'll use the existing soft-delete mechanism used today when a user edits a message.
3. Take the chat history after the last common message from the ACP session and insert it into the database. Ensure the first user message after a [new session marker](#new-session-marker)

### ACP dialer

The ACP dialer is a struct backed by a persistent goroutine spawned once per ACP runner that manages communication with the workspace agent and the ACP adapter within.

The dialer exposes the following methods:

- `StreamMessageParts(after_message_id string, history_version int, generation_attempt int) error`: streams message parts after the message identified by `after_message_id` from the ACP adapter and saves them to the message parts buffer episode identified by `history_version` and `generation_attempt`. Idempotent.
- `GetMessageParts(known_message_ids array<string>) (string, Message[], error)` returns the last common message id between known_message_ids and the message history in the ACP adapter, together with all the messages in the adapter after the last common message.
- `SubmitPrompt(prompt Message) error`: submit a prompt to the harness. The harness must be idle, otherwise this will return an error.

## Switching harnesses and workspaces

It's possible to switch the harness and workspace of an existing chat. If we're switching an ACP harness to the Coder Agents harness, the process is straighforward: just update the `harness` column on the chat row in the database. Since all messages use the same standard format, Coder Agents can take over ACP chats directly.

If we're switching from a Coder Agents or an ACP harness to a different ACP harness, it's a bit more complicated. ACP doesn't allow us to submit the message history for processing: we need to convert it into a prompt and submit it as a regular user message instead.

I see a couple of sensible ways of converting a history into a user message. We can:

- naively concatenate the history into a single message; or
- compact the history using an LLM and use the result as the prompt; or
- concatenate only the user and assistant messages without tool calls and results into a single message.

I expect that the naive concatenation approach wouldn't work very well. The message could be gigantic and overflow the size limits that harnesses impose on user messages. I wouldn't expect it be effective as an LLM prompt either: it's unlikely that LLMs are trained to respond well to prompts that are very long agent conversation transcripts.

We could make use of compaction, and with a good prompt it's likely to generate a high quality summary of the conversation. The downside is extra latency; compaction usually takes 10-20 seconds.

I believe the third option of only including user and assistant messages in the prompt provides a good middle ground. It should be enough context to continue the conversation in a new session, and it shouldn't be so long that it overwhelms the LLM.

In this RFC we will not develop a solution to the scenario when the prompt exceeds the message size limit of the selected harness. If it does, the harness will return an error, and the chat will move into an error state. Then the user will be able to either select another harness and try again, or start a new chat.

### First message after switching to an ACP harness

The prompt submitted to the agent will have the following format:

```md
<context>
[concatenated messages here]
</context>
<prompt>
[contents of the latest user message]
</prompt>
```

In the UI we will not want to display the full prompt to the user; the contents of the user message will be enough. To do that, we will append 2 messages to the chat history: one with the full prompt with visibility "model", and the other with just the user message with visibility "user". In that order.

### New session marker

We will need to add some kind of a marker in the message history to mark where a new ACP session starts. This will be needed by the [message sync procedure](#message-sync-procedure) to avoid overwriting messages from previous sessions. This can be an assistant message with visibility "user" representing a tool call to a custom tool like "new_session_started".

## Workspace agent

The workspace agent is responsible for managing ACP sessions. It will learn about what harnesses it can spawn from the [workspace agent manifest](https://github.com/coder/coder/blob/053895f8eee1b5a7641a4d87777e7f6e6d490f6d/coderd/agentapi/manifest.go#L138), where we will add a new `AgentsHarnesses` field. It maintains an in-memory registry of harnesses. Harnesses allow creating and interacting with sessions.

### Harness registry

The harness registry is a mapping of harness slugs to `Harness` structs. A `Harness` struct exposes the following methods:

- `NewSession() (*Session, error)`: creates a new session and returns a pointer to it.
- `GetSession(id string) (*Session, error)`: returns the session identified by `id`, or an error if it's not found. If it doesn't find the session in-memory, it will attempt to use ACP's `session/load` or `session/resume` based on what capabilities the harness advertises. If the harness doesn't advertise these capabilities at all, `GetSession` will return an error.

ACP is an async, message-based protocol. A `Harness` maintains a goroutine that manages a live connection to the harness. `Session`s are scoped to that connection: if the connection dies, the goroutine runs `Close()` on all sessions.

### Sessions

The `Session` struct exposes the following methods:

- `SubmitPrompt(chatd.Message) (error)`: submits a prompt to the session. The session must be idle, otherwise `SubmitPrompt` returns an error. The ACP adapter [advertises](https://github.com/agentclientprotocol/agent-client-protocol/blob/33925a28a89f7c32e9d406f875e520c169034d0e/docs/protocol/v1/initialization.mdx#L202) which content types it accepts in the prompt, so we need to convert the submitted prompt accordingly.
- `ConfigOptions() (ConfigOptions, error)`: returns the config options that [ACP advertises](https://github.com/agentclientprotocol/agent-client-protocol/blob/33925a28a89f7c32e9d406f875e520c169034d0e/docs/protocol/v1/session-config-options.mdx#L17).
- `Status() (SessionStatus, error)`: returns the status of the session. Can be either `running` or `idle`.
- `MessageHistory() (chatd.Message[], error)`
- `Close()`: closes the session. Only affects the in-memory representation of a session, it does not call the ACP adapter. After running `Close`, all other methods start returning a `closed` error.

### ACP config options discovery

After the workspace agent becomes `ready`, we start all configured harnesses and query them for config options. The workspace agent sends it to control plane, which saves it to the `acp_config_options` table. This procedure is done once per workspace agent startup.

### Initial message history sync

When the adapter is started, and it advertises the [`session/load` capability](https://github.com/agentclientprotocol/agent-client-protocol/blob/33925a28a89f7c32e9d406f875e520c169034d0e/docs/protocol/v1/initialization.mdx#L188), the workspace agent constructs an in-memory representation of the session history. If it doesn't advertise the capability we note it, and act as if `session/load` returned no messages.

We convert messages from ACP's format into chatd's `Message` format.

#### Message IDs

ACP returns optional ids on messages. Some adapters, like the ones for Claude Code and Codex, ensure these ids are persistent across adapter restarts. Other adapters only ensure ids are persistent within a single ACP session. There are also adapters that don't expose message ids at all.

For each ACP message that doesn't have an id, we generate a synthetic id by combining the message index and the hash of the contents of the message combined with the hash of the previous message: `{idx}-{hash}`. If there's no previous message, we use the the string `0` as a stand-in for the previous message hash.

### Delivering messages

The ACP adapter [advertises](https://github.com/agentclientprotocol/agent-client-protocol/blob/33925a28a89f7c32e9d406f875e520c169034d0e/docs/protocol/v1/initialization.mdx#L202) which content types it accepts in the prompt, so we need to convert the submitted prompt accordingly.
