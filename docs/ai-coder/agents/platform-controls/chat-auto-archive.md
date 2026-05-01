# Conversation Auto-Archive

Coder Agents automatically archives long-inactive conversations so they
drop out of active chat lists without any user intervention. Archived
conversations are still visible (and can be unarchived) until they age
out of the separate retention window, at which point they are purged.

## How it works

A background process runs approximately every 10 minutes. On each tick
it scans the chat database for root conversations whose most recent
non-deleted message is older than the configured auto-archive window
and flips them from "active" to "archived". Cascaded children (chats
linked into a larger conversation via `root_chat_id`) are archived
alongside their parent so the conversation stays coherent.

Activity is defined as the most recent non-deleted message in the
conversation family, counting messages from every role. Root chats
whose status indicates ongoing work (`running`, `pending`, `paused`,
or `requires_action`) are never selected for auto-archiving.
Children inherit their root's archival decision.

Pinned root conversations (those with a non-zero pin order) are never
selected for auto-archiving. Children are archived alongside their
root regardless of individual pin status. Admins and users who want
to retain a conversation long after its last message should pin the
root.

## Notifications

When your chats are auto-archived, you receive a digest notification
listing the titles of the archived conversations and the
auto-archive window currently configured.

If you find the digest noisy, you can disable the "Chats
Auto-Archived" notification entirely from your notification preferences.

## Interaction with retention

Auto-archive and deletion are two independent controls:

| Control             | What it does                                                              | Default           |
|---------------------|---------------------------------------------------------------------------|-------------------|
| Auto-archive window | Moves inactive chats to the archived state                                | 0 days (disabled) |
| Retention window    | Deletes chats that have been archived long enough and orphaned chat files | 30 days           |

A conversation needs to be inactive for `auto_archive_days`, then
archived for `retention_days`, before it is deleted. The two windows
stack additively. With auto-archive disabled by default, inactive
chats are never auto-archived; once an admin opts in by setting a
non-zero `auto_archive_days`, a conversation lives for at least
`auto_archive_days + retention_days` from its last message before it
is permanently removed.

Auto-archive (like manual archive) resets the per-chat retention
clock, so the full `retention_days` runs from the tick that archived
the chat, not from its last message.

Setting either value to `0` disables that step. Setting
`auto_archive_days` to `0` means inactive chats are never
auto-archived (users still archive manually). Setting
`retention_days` to `0` means archived chats are kept indefinitely.

## Configuration

The auto-archive window is stored as the
`agents_chat_auto_archive_days` key in the `site_configs` table.
The default is `0` (disabled); set to a positive number of days to
enable auto-archiving.

Use the admin API to read or update the value:

    GET  /api/experimental/chats/config/auto-archive-days
    PUT  /api/experimental/chats/config/auto-archive-days

## Rollout advice

Auto-archive is disabled by default, so upgrading to a release that
includes this feature will not archive any existing chats until an
admin opts in. The first tick after enabling auto-archive on a
deployment with a long history will process up to 1,000 root chats
(and their children). If your deployment has a large backlog, the
initial rollout will span many ticks. This is intentional and avoids
stalling the rest of `dbpurge` during the first run. To disable,
set `auto_archive_days` back to `0`.

## Audit trail

Each auto-archived root chat produces an audit log entry with the
background subsystem tag `chat_auto_archive`. Cascaded children are
not audited individually.
