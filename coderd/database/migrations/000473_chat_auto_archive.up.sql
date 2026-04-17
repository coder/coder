-- Per-owner dedupe record for the chat auto-archive digest
-- notification. Presence of a row indicates a digest was sent to the
-- owner; dbpurge skips re-sending until last_sent_at is older than
-- the dedupe window (24 h).
CREATE TABLE chat_auto_archive_digest_log (
    owner_id     UUID        PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    last_sent_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE chat_auto_archive_digest_log IS 'Per-owner dedupe record for the chat auto-archive digest notification. Presence of a row indicates a digest was sent to the owner; dbpurge skips re-sending until last_sent_at is older than the dedupe window (24 h).';

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
-- digest of auto-archived chats.
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
    E'Hi {{.UserName}},\n\nThe following chat{{if ne (len .Data.archived_chats) 1}}s were{{else}} was{{end}} automatically archived because {{if ne (len .Data.archived_chats) 1}}they have{{else}}it has{{end}} been inactive for more than {{.Data.auto_archive_days}} days:\n\n{{range .Data.archived_chats}}* "{{.title}}" (last active {{.last_activity_humanized}})\n{{end}}\nYou can restore any of them from the Chats page within {{.Data.retention_days}} days, after which they will be permanently deleted.',
    'Chat Events',
    '[
        {
            "label": "View archived chats",
            "url": "{{ base_url }}/chats?archived=true"
        }
    ]'::jsonb
);
