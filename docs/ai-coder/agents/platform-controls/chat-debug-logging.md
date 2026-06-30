# Chat debug logging

Records a detailed trace of each chat turn for troubleshooting: the
normalized request sent to the LLM provider, the full response, token usage,
retry attempts, and errors.

Off by default. Three layers control whether it runs for a given chat:

1. **Deployment override.** Setting `CODER_CHAT_DEBUG_LOGGING_ENABLED=true`
   (or `--chat-debug-logging-enabled` at server start) forces debug logging
   on for every chat. The runtime admin and user toggles become read-only.
1. **Runtime admin gate.** With the deployment override unset, the
   *Let users record chat debug logs* toggle decides whether users can opt
   in. Configure it under **AI Settings** > **Lifecycle**, or at
   `GET/PUT /api/experimental/chats/config/debug-logging`.
1. **Per-user toggle.** Users with the admin gate enabled can turn debug
   logging on for their own chats from **Agents** > **Settings** > **General**
   under *Record debug logs for my chats*. The endpoint
   `PUT /api/experimental/chats/config/user-debug-logging` returns
   `409 Conflict` if the deployment override is active and `403 Forbidden`
   if the admin has not enabled user opt-in.

> [!IMPORTANT]
> Debug logs may contain sensitive content from prompts, responses, tool
> calls, and errors. Treat them with the same care as conversation history.
> Only the chat owner (or a user with read access to the chat) can fetch a
> chat's debug runs through the API. Administrators do not get blanket
> access to all users' debug data.

When debug logging is active for a chat, a **Debug** tab appears in the
right panel of the Agents page (alongside Git, Terminal, and Desktop) for
that chat's owner. The tab lists recent debug runs and lets you expand a run
into its per-step request, response, token usage, retry attempts, errors,
and policy metadata.

## Export debug logs

You can export the same captured debug data from the UI:

1. Navigate to **Agents**.
1. Open a chat with debug logging enabled.
1. Open the **Debug** tab in the right panel.
1. Click **Export debug logs** to download the chat's recent debug runs as
   JSON, or expand a run and click **Export this run** to download one run.

The chat-level export includes the full run detail for the runs returned by
the debug run list endpoint. The current list endpoint returns up to 100 of
the newest runs.

### API access

The same data is available through the experimental API:

- `GET /api/experimental/chats/{chat}/debug/runs` lists the most recent runs
  for a chat (up to 100, newest first).
- `GET /api/experimental/chats/{chat}/debug/runs/{debugRun}` returns a single
  run with all of its steps, including normalized request and response bodies.

Fetch a single run and save it as JSON:

```sh
export CODER_URL="https://coder.example.com"
export CODER_SESSION_TOKEN="$(coder login token)"
export CHAT_ID="00000000-0000-0000-0000-000000000000"
export RUN_ID="11111111-1111-1111-1111-111111111111"

curl -fsS \
  -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
  "$CODER_URL/api/experimental/chats/$CHAT_ID/debug/runs/$RUN_ID" \
  | jq . > "coder-agents-debug-run-$RUN_ID.json"
```

Fetch every run returned by the list endpoint and save a chat-level export.
Using the same `CODER_URL`, `CODER_SESSION_TOKEN`, and `CHAT_ID` variables
from above:

```sh
RUN_IDS=$(curl -fsS \
  -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
  "$CODER_URL/api/experimental/chats/$CHAT_ID/debug/runs" \
  | jq -r '.[].id') || {
  echo "Failed to list debug runs" >&2
  exit 1
}

RUN_EXPORTS=$(mktemp)
trap 'rm -f "$RUN_EXPORTS"' EXIT

for RUN_ID in $RUN_IDS; do
  curl -fsS \
    -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
    "$CODER_URL/api/experimental/chats/$CHAT_ID/debug/runs/$RUN_ID" \
    >> "$RUN_EXPORTS" || {
      echo "Failed to fetch debug run $RUN_ID" >&2
      exit 1
    }
  echo >> "$RUN_EXPORTS"
done

jq -s \
  --arg chat_id "$CHAT_ID" \
  --arg exported_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  '{
    version: 1,
    scope: "chat",
    exported_at: $exported_at,
    chat_id: $chat_id,
    run_count: length,
    limited_to_most_recent: 100,
    runs: .
  }' "$RUN_EXPORTS" > "coder-agents-debug-chat-$CHAT_ID.json"
```

Debug runs are stored alongside the chat and are removed when the parent
conversation is deleted (manually, by retention, or by chat purge). See
[Data Retention](./chat-retention.md) for the conversation retention
controls.
