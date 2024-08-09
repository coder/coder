UPDATE notification_templates
SET body_template = E'Hi {{.UserName}}\n' ||
                    E'Your workspace **{{.Labels.name}}** has been updated automatically to the latest template version ({{.Labels.template_version_name}}).'
WHERE id = 'c34a0c09-0704-4cac-bd1c-0c0146811c2b';