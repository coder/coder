-- Partial index supporting the AutoArchiveInactiveChats CTE predicate.
-- Auto-archive only considers active (archived = false), unpinned
-- (pin_order = 0) root chats (parent_chat_id IS NULL), so a partial
-- index lets dbpurge jump straight to candidates without scanning the
-- full chats table even in deployments with millions of archived or
-- cascaded chats.
CREATE INDEX IF NOT EXISTS idx_chats_auto_archive_candidates
    ON chats (created_at)
    WHERE archived = false
      AND pin_order = 0
      AND parent_chat_id IS NULL;

-- Notification template used by dbpurge to send the per-owner
-- digest of auto-archived chats. Per-owner deduplication is handled
-- by the native notification_messages dedupe hash (template_id,
-- user_id, payload, day); users who find the digest noisy can
-- disable this template in their notification preferences.
--
-- The "Hi {{.UserName}}," greeting is prepended by the SMTP and
-- webhook wrappers, so the body_template must not repeat it. No
-- action buttons are included; archived chats aren't yet visible
-- in the frontend, and adding that view is out of scope here.
INSERT INTO notification_templates (
    id,
    name,
    title_template,
    body_template,
    "group",
    actions
)
VALUES (
    '764031be-4863-4220-867b-6ce1a1b7a5f5',
    'Chats Auto-Archived',
    E'Chats auto-archived after {{.Data.auto_archive_days}} days of inactivity',
    E'The following chat{{if ne (len .Data.archived_chats) 1}}s were{{else}} was{{end}} automatically archived because {{if ne (len .Data.archived_chats) 1}}they have{{else}}it has{{end}} been inactive for more than {{.Data.auto_archive_days}} days:\n\n{{range .Data.archived_chats}}* "{{.title}}" (last active {{.last_activity_humanized}})\n{{end}}{{with .Data.additional_archived_count}}...and {{.}} more.\n\n{{end}}\nYou can restore any of them from the Chats page within {{.Data.retention_days}} days, after which they will be permanently deleted.',
    'Chat Events',
    '[]'::jsonb
);
