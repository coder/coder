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
conversation family, counting messages from every role. If an agent is
still generating a response, the chat counts as active even if the
human-authored message is old.

Pinned conversations (those with a non-zero pin order) are never
auto-archived. Admins and users who want to retain a conversation long
after its last message should pin it.

## Notifications

The first time one of your chats is auto-archived in a given 24-hour
window, you receive a single digest notification listing the titles
of the archived conversations and the auto-archive window currently
configured. Subsequent archives inside the same window are absorbed
into the next digest rather than generating more notifications.

This notification is sent via the standard notifications subsystem,
so delivery respects each user's notification preferences and the
configured SMTP/webhook dispatcher.

## Interaction with retention

Auto-archive and deletion are two independent controls:

| Control             | What it does                                                              | Default |
|---------------------|---------------------------------------------------------------------------|---------|
| Auto-archive window | Moves inactive chats to the archived state                                | 90 days |
| Retention window    | Deletes chats that have been archived long enough and orphaned chat files | 30 days |

A conversation needs to be inactive for `auto_archive_days`, then
archived for `retention_days`, before it is deleted. The two windows
stack additively, so with defaults a conversation lives for at least
120 days from its last message before it is permanently removed.

Setting either value to `0` disables that step. Setting
`auto_archive_days` to `0` means inactive chats are never
auto-archived (users still archive manually). Setting
`retention_days` to `0` means archived chats are kept indefinitely.

## Configuration

Navigate to the **Agents** page, open **Settings**, and select the
**Behavior** tab to configure the auto-archive window. The default is
90 days. Use the toggle to disable auto-archive entirely.

The value is stored as the `agents_chat_auto_archive_days` key in
the `site_configs` table and can also be managed via the API at
`/api/experimental/chats/config/auto-archive-days`.

## Rollout advice

The first tick after enabling auto-archive on a deployment with a
long history will process up to 1,000 root chats (and their
children). If your deployment has a large backlog, the initial rollout
will span many ticks — this is intentional and avoids stalling the
rest of `dbpurge` during the first run. If you need to prevent the
initial backfill entirely during a migration window, set
`auto_archive_days = 0` before upgrading and re-enable it when you
are ready.

## Audit trail

Each auto-archived chat produces an audit log entry with the
background subsystem tag `chat_auto_archive`. The audit entry records
the chat ID, owner ID, and organization ID, and the diff shows
`archived: false → true`.
