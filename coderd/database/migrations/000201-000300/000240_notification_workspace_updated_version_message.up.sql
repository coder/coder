UPDATE notification_templates
SET name          = 'Workspace Updated Automatically', -- drive-by fix for capitalization to match other templates
    body_template = E'Hi {{.UserName}}\n' ||
                    E'Your workspace **{{.Labels.name}}** has been updated automatically to the latest template version ({{.Labels.template_version_name}}).\n' ||
                    E'Reason for update: **{{.Labels.template_version_message}}**' -- include template version message
WHERE id = 'c34a0c09-0704-4cac-bd1c-0c0146811c2b';