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
        'Template Build Failure',
        E'Build failed on workspace using template "{{.Labels.name}}"',
        E'Hi {{.UserName}},\n\n'
        'A workspace using the template **{{.Labels.name}}** failed to build.\n\n'
        '- **Version**: {{.Labels.version}}\n'
        '- **Workspace**: {{.Labels.workspaceName}}\n'
        '- **Transition**: {{.Labels.transition}}\n'
        '{{if .Labels.initiator}}- **Initiated by**: {{.Labels.initiator}}{{end}}'
        '{{if .Labels.reason}}- **Reason**: {{.Labels.reason}}{{end}}'
        '\n\nYou can debug this workspace using the build logs below or contact the deployment administrator.',
        'Template Events',
        '[
        {
            "label": "View build",
            "url": "{{ base_url }}/@{{.Labels.workspaceOwnerName}}/{{.Labels.workspaceName}}/builds/{{.Labels.buildNumber}}"
        },
        {
            "label": "View template",
            "url": "{{ base_url }}/templates/{{.Labels.name}}"
        }
    ]'::jsonb
    );
