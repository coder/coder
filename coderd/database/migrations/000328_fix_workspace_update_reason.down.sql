UPDATE notification_templates
SET body_template = E'Your workspace **{{.Labels.name}}** has been updated automatically to the latest template version ({{.Labels.template_version_name}}).\n\n' ||
					E'Reason for update: **{{.Labels.template_version_message}}**.'
WHERE id = 'c34a0c09-0704-4cac-bd1c-0c0146811c2b';
