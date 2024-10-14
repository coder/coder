UPDATE notification_templates
SET
	body_template = E'Hi {{.UserName}},\n\n' ||
					E'A manual build of the workspace **{{.Labels.name}}** using the template **{{.Labels.template_name}}** failed (version: **{{.Labels.template_version_name}}**).\n\n' ||
					-- Mention template display name:
					E'The template''s display name was **{{.Labels.template_display_name}}**. ' ||
					E'The workspace build was initiated by **{{.Labels.initiator}}**.'
WHERE
	id = '2faeee0f-26cb-4e96-821c-85ccb9f71513';
