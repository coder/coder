-- Template for the per-owner chat auto-archive notification. Enqueue is
-- per-tick (see dbpurge.dispatchChatAutoArchive): owners whose backlog
-- spans multiple ticks receive multiple notifications, and
-- notification_messages dedupe does not collapse them because each
-- tick's payload differs. Users who find this noisy can disable the
-- template from their notification preferences. The SMTP/webhook
-- wrappers prepend "Hi {{.UserName}},", so body_template must not.
INSERT INTO notification_templates (
    id,
    name,
    title_template,
    body_template,
    actions,
    "group",
    method,
    kind,
    enabled_by_default
)
VALUES (
    '764031be-4863-4220-867b-6ce1a1b7a5f5',
    'Chats Auto-Archived',
    E'Chats auto-archived after {{.Data.auto_archive_days}} days of inactivity',
    E'The following chats were automatically archived:\n\n{{range .Data.archived_chats}}* "{{.title}}" (last active {{.last_activity_humanized}})\n{{end}}{{with .Data.additional_archived_count}}\n...and {{.}} more.\n\n{{end}}\n{{if eq .Data.retention_days "0"}}You can restore any of them from the Agents page; archived chats are kept indefinitely.{{else}}You can restore any of them from the Agents page within {{.Data.retention_days}} days, after which they will be permanently deleted.{{end}}',
    '[
        {
            "label": "View chats",
            "url": "{{base_url}}/agents?archived=archived"
        }
    ]'::jsonb,
    'Chat Events',
    NULL,
    'system'::notification_template_kind,
    true
);
