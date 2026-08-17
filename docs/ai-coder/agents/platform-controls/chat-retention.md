# Conversation Data Retention

Coder Agents automatically cleans up old conversation data to manage database
growth. Archived conversations and their associated files are periodically
purged based on a configurable retention period.

Conversations become eligible for purging only after they are archived. Old
conversations can be archived manually, or automatically. See
[Auto-Archive](./chat-auto-archive.md) for how the two controls interact.

Debug run and step cleanup is controlled separately. See
[Chat Debug Data Retention](./chat-debug-retention.md).

## How it works

A background process runs approximately every 10 minutes to remove expired
conversation data. Only archived conversations are eligible for deletion —
active (non-archived) conversations are never purged.

When an archived conversation exceeds the retention period, Coder deletes it along with its messages, diff statuses, and queued messages.
Coder retains an attached file while any conversation references it, regardless of whether the conversation is active or archived.
A file that exceeds the retention period becomes eligible for deletion only after no conversations reference it.
Conversation and file cleanup operations run in batches of 1,000 rows per cycle.

## Configuration

Navigate to the **Agents** page, open **Settings**, and select the **Behavior**
tab to configure the conversation retention period. The default is 30 days. Use the toggle to
disable retention entirely.

Use the admin API to read or update the value:

```txt
GET  /api/v2/chats/config/retention-days
PUT  /api/v2/chats/config/retention-days
```

## What gets deleted

| Data                   | Condition                                                          | Cascade                                                       |
|------------------------|--------------------------------------------------------------------|---------------------------------------------------------------|
| Archived conversations | Archived longer than retention period                              | Messages, diff statuses, queued messages deleted via CASCADE. |
| Conversation files     | Older than retention period and not referenced by any conversation | None                                                          |

## Unarchive safety

Archiving a conversation does not make its attached files eligible for deletion.
If you unarchive a conversation before Coder purges it, its attachments remain available, even when the files exceed the retention period.
