INSERT INTO
    notification_templates (
        id,
        name,
        title_template,
        body_template,
        "group",
        actions
    )
VALUES (
        '48a9d2b9-3655-430c-a31a-2442479e7519',
        'Template Manual Build Failure',
        E'Workspace with template "{{.Labels.name}}" failed to build',
        E'Hi {{.UserName}}\n\nThe workspace **{{.Labels.workspaceName}}**, using the template **{{.Labels.name}}**, failed during a manual build({{.Labels.transition}}) initiated by the user **{{.Labels.initiator}}**.',
        'Template Events',
        '[
        {
            "label": "View build",
            "url": "{{ base_url }}/@{{.Labels.workspaceUserName}}/{{.Labels.workspaceName}}/builds/{{.Labels.buildNumber}}"
        },
		{
            "label": "View Template",
            "url": "{{ base_url }}/templates/{{.Labels.name}}"
        }
    ]'::jsonb
    );
