# Chats API

> [!NOTE]
> The Chats API is experimental and gated behind the `agents` experiment flag.
> Endpoints live under `/api/experimental/chats` and may change without notice.

The Chats API lets you create and interact with Coder Agents
programmatically. You can start a chat, send follow-up messages, and stream
the agent's response — all without using the Coder dashboard.

## Authentication

All endpoints require a valid session token:

```sh
curl -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
  https://coder.example.com/api/experimental/chats
```

## Quick start

Create a chat with a single text prompt:

```sh
curl -X POST https://coder.example.com/api/experimental/chats \
  -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "content": [
      {"type": "text", "text": "hello world"}
    ]
  }'
```

The response is the newly created `Chat` object:

```json
{
  "id": "a1b2c3d4-...",
  "owner_id": "...",
  "workspace_id": null,
  "last_model_config_id": "...",
  "title": "hello world",
  "status": "waiting",
  "last_error": null,
  "created_at": "2025-07-17T00:00:00Z",
  "updated_at": "2025-07-17T00:00:00Z",
  "archived": false
}
```

The agent begins processing the prompt asynchronously. Use the
[stream endpoint](#stream-updates) to follow its progress.

## Core workflow

A typical integration follows three steps:

1. **Create a chat** — `POST /api/experimental/chats` with your prompt.
2. **Stream updates** — Open a WebSocket to
   `GET /api/experimental/chats/{chat}/stream` to receive real-time events
   as the agent works.
3. **Send follow-ups** — `POST /api/experimental/chats/{chat}/messages` to
   add messages to the conversation. Messages are queued if the agent is
   busy.

## Endpoints

### Create a chat

`POST /api/experimental/chats`

| Field             | Type              | Required | Description                                     |
|-------------------|-------------------|----------|-------------------------------------------------|
| `content`         | `ChatInputPart[]` | yes      | The user's prompt as one or more content parts. |
| `workspace_id`    | `uuid`            | no       | Pin the chat to a specific workspace.           |
| `model_config_id` | `uuid`            | no       | Override the default model configuration.       |

Each `ChatInputPart` has a `type` field. The simplest form is a text part:

```json
{"type": "text", "text": "Fix the failing tests in the auth service"}
```

Other part types include `file` (an uploaded image referenced by its
`file_id`) and `file-reference` (a pointer to a file with optional line
range).

**Response**: `201 Created` with a `Chat` object.

### Send a message

`POST /api/experimental/chats/{chat}/messages`

| Field             | Type              | Required | Description                       |
|-------------------|-------------------|----------|-----------------------------------|
| `content`         | `ChatInputPart[]` | yes      | The follow-up message content.    |
| `model_config_id` | `uuid`            | no       | Override the model for this turn. |

If the agent is currently processing, the message is queued automatically.
The response indicates whether the message was delivered immediately or
queued:

```json
{
  "queued": false,
  "message": { "id": 42, "chat_id": "...", "role": "user", "created_at": "...", "content": [...] }
}
```

When `queued` is `true`, `message` is absent and `queued_message` is
returned instead.

### Stream updates

`GET /api/experimental/chats/{chat}/stream`

Opens a **one-way WebSocket** connection. The server sends events; clients
must not write to the socket (doing so closes the connection).

| Query parameter | Type    | Required | Description                               |
|-----------------|---------|----------|-------------------------------------------|
| `after_id`      | `int64` | no       | Only return events after this message ID. |

Each WebSocket message is a JSON envelope with an outer `type`
(`"ping"`, `"data"`, or `"error"`) and an optional `data` field. For
`"data"` envelopes the payload is a **JSON array** of event objects:

```json
{
  "type": "data",
  "data": [
    {"type": "status", "chat_id": "...", "status": {"status": "running"}},
    {"type": "message_part", "chat_id": "...", "message_part": {"...":"..."}}
  ]
}
```

Ignore `"ping"` envelopes (keepalives sent every ~15 s). On first
connect the server sends an initial snapshot of the chat state before
switching to live events. Use `after_id` when reconnecting to skip
messages the client already has.

Event types inside each batch:

| Type           | Description                                                  |
|----------------|--------------------------------------------------------------|
| `message_part` | A chunk of the agent's response (text, tool call, etc.).     |
| `message`      | A complete message has been persisted.                       |
| `status`       | The chat status changed (e.g. `running`, `waiting`).         |
| `error`        | An error occurred during processing.                         |
| `retry`        | The server is retrying a failed LLM call (includes backoff). |
| `queue_update` | The queued message list changed.                             |

### List chats

`GET /api/experimental/chats`

Returns all chats owned by the authenticated user.

### Get a chat

`GET /api/experimental/chats/{chat}`

Returns the `Chat` object (metadata only, no messages).

### Get chat messages

`GET /api/experimental/chats/{chat}/messages`

Returns the messages and queued messages for a chat.

### List models

`GET /api/experimental/chats/models`

Returns available models. Use this to discover valid values for
`model_config_id`.

### Archive / unarchive

`POST /api/experimental/chats/{chat}/archive`
`POST /api/experimental/chats/{chat}/unarchive`

Archive hides a chat from the default list without deleting it.

### Interrupt

`POST /api/experimental/chats/{chat}/interrupt`

Stops the agent's current processing loop and returns the chat to
`waiting` status.

## File uploads

Attach images to a chat by uploading them first:

```sh
curl -X POST "https://coder.example.com/api/experimental/chats/files?organization=$ORG_ID" \
  -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
  -H "Content-Type: image/png" \
  --data-binary @screenshot.png
```

The response contains an `id` you can reference as `file_id` in a
`ChatInputPart` with `"type": "file"`. To retrieve a previously uploaded
file, use `GET /api/experimental/chats/files/{file}`.

Supported formats: PNG, JPEG, GIF, WebP (up to 10 MB). The server
validates actual file content regardless of the declared `Content-Type`.

## Chat statuses

| Status    | Meaning                                                      |
|-----------|--------------------------------------------------------------|
| `waiting` | Idle — newly created, finished successfully, or interrupted. |
| `pending` | Queued for processing.                                       |
| `running` | Agent is actively working.                                   |
| `error`   | Agent encountered an error.                                  |
