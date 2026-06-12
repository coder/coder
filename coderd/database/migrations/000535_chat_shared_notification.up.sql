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
    'b789bd75-d7c6-4cab-9757-1147ab184903',
    'Chat Shared',
    E'{{.Labels.initiator}} shared a chat with you',
    E'{{.Labels.initiator}} shared the chat "**{{.Labels.chat_title}}**" with you.',
    '[
        {
            "label": "View chat",
            "url": "{{base_url}}/agents/{{.Labels.chat_id}}"
        }
    ]'::jsonb,
    'Chat Events',
    NULL,
    'system'::notification_template_kind,
    true
);
