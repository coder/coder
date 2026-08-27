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
    'd2209b6a-3ac7-4560-8de3-cc9024cc5708',
    'MCP Tool Call Awaiting Approval',
    E'MCP tool "{{.Labels.tool}}" on {{.Labels.server_slug}} is awaiting approval',
    E'A call to tool **{{.Labels.tool}}** on MCP server **{{.Labels.server_slug}}** in workspace **{{.Labels.workspace_name}}** is held awaiting your approval.',
    '[
        {
            "label": "Review request",
            "url": "{{base_url}}/mcp-escalations"
        }
    ]'::jsonb,
    'MCP Gateway Events',
    NULL,
    'system'::notification_template_kind,
    true
);
