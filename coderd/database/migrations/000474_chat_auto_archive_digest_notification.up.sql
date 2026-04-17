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
