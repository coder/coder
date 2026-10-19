---
title: Structured output
---

Structured output is experimental. It asks a chat for a final answer that matches a JSON Schema.
The turn ends with a receipt: a message that holds either a value that satisfies the schema or a typed failure.

## Enable the experiment

```sh
coder server --experiments=chat-structured-output
```

Enable the experiment only after every replica runs a release that supports it.
Before you downgrade, wait until no structured output request is pending.
Turning the experiment off rejects new requests; requests already accepted still finish, and their receipts stay readable.

## Create a chat that returns structured output

Add `response_format` to the body of `POST /api/v2/chats`:

```json
{
  "organization_id": "<organization-id>",
  "content": [{ "type": "text", "text": "Count the open incidents." }],
  "response_format": {
    "type": "json_schema",
    "json_schema": {
      "name": "incident_count",
      "description": "The number of open incidents.",
      "schema": { "type": "object", "properties": { "total": { "type": "integer" } }, "required": ["total"] }
    }
  }
}
```

The server rejects unknown fields in `response_format`, including `strict`, with HTTP 400 and the exact field in `validations`.
Omit `response_format` or send `{"type": "text"}` for an ordinary chat; structured output can't be combined with plan mode.

## Send a message that returns structured output

Add the same `response_format` to the body of `POST /api/v2/chats/{chat}/messages`:

- An idle chat inserts the message, and `message.structured_output_request_id` holds the request ID.
- A busy chat queues the message, and `queued_message.structured_output_request_id` holds the request ID. The message keeps it when the queue promotes it.
- With `"busy_behavior": "interrupt"`, the current turn stops and the formatted message runs after it with its own receipt. A structured output request of the stopped turn gets a `canceled` receipt with `error.code` `interrupted`.
- Deleting a queued formatted message produces a `canceled` receipt with `error.code` `queue_deleted`.

A format returns HTTP 400 on child chats, chats with a mode, chats in plan mode, and sends that set `plan_mode` to `plan`.

## Edit a message that returns structured output

`PATCH /api/v2/chats/{chat}/messages/{message}` also accepts `response_format`:

- Omitted or JSON `null` keeps the edited message's format under a new request ID; the server compiles the stored schema again under the current limits.
- `{"type": "text"}` makes the replacement ordinary text.
- A `json_schema` format replaces the format under a new request ID.

An edit discards the edited message, every later message, and the queue.
Each discarded request that is still open gets a `canceled` receipt with `error.code` `superseded`.
With the experiment off, a `json_schema` format returns HTTP 400, and so does keeping the format of a message that asks for structured output; send `{"type": "text"}` instead.
The chat rules for sending apply to a kept or new format.

## Read the result

The user message that asks for structured output carries `structured_output_request_id`.
The receipt is an assistant message whose `structured_output.request_id` matches it, with one of these statuses:

- `succeeded`: `value` holds the output, which can be JSON `null`.
- `failed`: `error.code` is `not_produced`, `validation_exhausted`, `generation_failed`, or `configuration_error`.
- `canceled`: `error.code` is `interrupted`, `superseded`, or `queue_deleted`.

The receipt's `content` holds a text fallback for clients that don't read `structured_output`.
After a reconnect, upsert messages by `id` and match receipts by request ID, not by position.

## Limits

| Limit                       | Value                                                                                   |
|-----------------------------|-----------------------------------------------------------------------------------------|
| Schema document             | 16 KiB, 16 levels deep                                                                  |
| Output value                | 64 KiB, 32 levels deep, 4096 values (256 with `patternProperties`), 256 items per array |
| Number in the stored output | 128 characters as a plain decimal, so `1e200` is rejected                               |
| Rejected attempts           | 3: the first attempt and two repairs, then the request fails                            |

Enabling plan mode while a structured output request is pending returns HTTP 409.
The `regex` format isn't supported, and `const`, `enum`, and `uniqueItems` compare numbers at float64 precision.
