# Chat Data Retention

Coder Agents automatically cleans up old chat data to manage database growth.
Archived chats and their associated files are periodically purged based on a
configurable retention period.

## How it works

A background process runs approximately every 10 minutes to remove expired chat
data. Only archived chats are eligible for deletion — active (non-archived)
chats are never purged.

When an archived chat exceeds the retention period, it is deleted along with its
messages, diff statuses, and queued messages via cascade. Orphaned chat files
(not referenced by any active or recently-archived chat) are also deleted. Both
operations run in batches of 1,000 rows per cycle.

## Configuration

Navigate to **Deployment Settings** > **Agents** > **Behavior** to configure the
chat retention period. The default is 30 days. Set the value to `0` to disable
chat purging entirely.

The retention period is stored as the `agents_chat_retention_days` key in the
`site_configs` table and can also be managed via the API at
`/api/experimental/chats/config/retention-days`.

## What gets deleted

| Data           | Condition                                                                              | Cascade                                                       |
|----------------|----------------------------------------------------------------------------------------|---------------------------------------------------------------|
| Archived chats | Archived longer than retention period                                                  | Messages, diff statuses, queued messages deleted via CASCADE. |
| Chat files     | Older than retention period AND not referenced by any active or recently-archived chat | —                                                             |

## Unarchive safety

If a user unarchives a chat whose files were purged, stale file references are
automatically scrubbed. The chat remains usable but previously attached files
are no longer available.

## Related links

- [Coder Agents](./index.md)
- [Data Retention](../../admin/setup/data-retention.md)
