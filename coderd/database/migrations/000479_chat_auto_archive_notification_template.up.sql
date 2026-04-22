-- Template for the per-owner auto-archive digest. Dedupe happens via
-- the native notification_messages hash; users can disable this
-- template in their notification preferences. The SMTP/webhook
-- wrappers prepend "Hi {{.UserName}},", so body_template must not.
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
    E'The following chat{{if ne (len .Data.archived_chats) 1}}s were{{else}} was{{end}} automatically archived because {{if ne (len .Data.archived_chats) 1}}they have{{else}}it has{{end}} been inactive for more than {{.Data.auto_archive_days}} days:\n\n{{range .Data.archived_chats}}* "{{.title}}" (last active {{.last_activity_humanized}})\n{{end}}{{with .Data.additional_archived_count}}...and {{.}} more.\n\n{{end}}\n{{if eq .Data.retention_days "0"}}You can restore any of them from the Chats page; archived chats are kept indefinitely.{{else}}You can restore any of them from the Chats page within {{.Data.retention_days}} days, after which they will be permanently deleted.{{end}}',
    'Chat Events',
    '[]'::jsonb
);
