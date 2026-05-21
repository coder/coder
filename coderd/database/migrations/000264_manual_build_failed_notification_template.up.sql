UPDATE notification_templates
SET
	body_template = E'Hi {{.UserName}},\n\n' ||
					-- Revert to a single label for the template name:
					E'A manual build of the workspace **{{.Labels.name}}** using the template **{{.Labels.template_name}}** failed (version: **{{.Labels.template_version_name}}**).\n\n' ||
					E'The workspace build was initiated by **{{.Labels.initiator}}**.'
WHERE
	id = '2faeee0f-26cb-4e96-821c-85ccb9f71513';

UPDATE notification_templates
SET
	body_template = E'Hi {{.UserName}},\n\n' ||
					-- Revert to a single label for the template name:
					E'The template **{{.Labels.name}}** was deleted by **{{ .Labels.initiator }}**.\n\n'
WHERE
	id = '29a09665-2a4c-403f-9648-54301670e7be';
