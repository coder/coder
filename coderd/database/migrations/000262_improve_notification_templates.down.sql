UPDATE notification_templates
SET
	body_template = E'Hi {{.UserName}},\nUser account **{{.Labels.suspended_account_name}}** has been suspended.'
WHERE
	id = 'b02ddd82-4733-4d02-a2d7-c36f3598997d';

UPDATE notification_templates
SET
	body_template = E'Hi {{.UserName}},\nYour account **{{.Labels.suspended_account_name}}** has been suspended.'
WHERE
	id = '6a2f0609-9b69-4d36-a989-9f5925b6cbff';

UPDATE notification_templates
SET
	body_template = E'Hi {{.UserName}},\nUser account **{{.Labels.activated_account_name}}** has been activated.'
WHERE
	id = '9f5af851-8408-4e73-a7a1-c6502ba46689';

UPDATE notification_templates
SET
	body_template = E'Hi {{.UserName}},\nYour account **{{.Labels.activated_account_name}}** has been activated.'
WHERE
	id = '1a6a6bea-ee0a-43e2-9e7c-eabdb53730e4';

UPDATE notification_templates
SET
	body_template = E'Hi {{.UserName}},\n\New user account **{{.Labels.created_account_name}}** has been created.'
WHERE
	id = '4e19c0ac-94e1-4532-9515-d1801aa283b2';

UPDATE notification_templates
SET
	body_template = E'Hi {{.UserName}},\n\nUser account **{{.Labels.deleted_account_name}}** has been deleted.'
WHERE
	id = 'f44d9314-ad03-4bc8-95d0-5cad491da6b6';

UPDATE notification_templates
SET
	body_template = E'Hi {{.UserName}}\n\n' ||
					E'The template **{{.Labels.name}}** was deleted by **{{ .Labels.initiator }}**.'
WHERE
	id = '29a09665-2a4c-403f-9648-54301670e7be';

UPDATE notification_templates
SET body_template = E'Hi {{.UserName}}\n' ||
					E'Your workspace **{{.Labels.name}}** has been updated automatically to the latest template version ({{.Labels.template_version_name}}).\n' ||
					E'Reason for update: **{{.Labels.template_version_message}}**'
WHERE
	id = 'c34a0c09-0704-4cac-bd1c-0c0146811c2b';

UPDATE notification_templates
SET
	body_template = E'Hi {{.UserName}}\n\nYour workspace **{{.Labels.name}}** was deleted.\nThe specified reason was "**{{.Labels.reason}}{{ if .Labels.initiator }} ({{ .Labels.initiator }}){{end}}**".'
WHERE
	id = '381df2a9-c0c0-4749-420f-80a9280c66f9';

UPDATE notification_templates
SET
	body_template = E'Hi {{.UserName}}\n\nYour workspace **{{.Labels.name}}** was deleted.\nThe specified reason was "**{{.Labels.reason}}{{ if .Labels.initiator }} ({{ .Labels.initiator }}){{end}}**".'
WHERE
	id = 'f517da0b-cdc9-410f-ab89-a86107c420ed';

UPDATE notification_templates
SET
	body_template = E'Hi {{.UserName}}\n\n' ||
		E'Your workspace **{{.Labels.name}}** has been marked as [**dormant**](https://coder.com/docs/templates/schedule#dormancy-threshold-enterprise) because of {{.Labels.reason}}.\n' ||
		E'Dormant workspaces are [automatically deleted](https://coder.com/docs/templates/schedule#dormancy-auto-deletion-enterprise) after {{.Labels.timeTilDormant}} of inactivity.\n' ||
		E'To prevent deletion, use your workspace with the link below.'
WHERE
	id = '0ea69165-ec14-4314-91f1-69566ac3c5a0';

UPDATE notification_templates
SET
	body_template = E'Hi {{.UserName}}\n\n' ||
		E'Your workspace **{{.Labels.name}}** has been marked for **deletion** after {{.Labels.timeTilDormant}} of [dormancy](https://coder.com/docs/templates/schedule#dormancy-auto-deletion-enterprise) because of {{.Labels.reason}}.\n' ||
		E'To prevent deletion, use your workspace with the link below.'
WHERE
	id = '51ce2fdf-c9ca-4be1-8d70-628674f9bc42';

UPDATE notification_templates
SET
	body_template = E'Hi {{.UserName}},\n\nA manual build of the workspace **{{.Labels.name}}** using the template **{{.Labels.template_name}}** failed (version: **{{.Labels.template_version_name}}**).\nThe workspace build was initiated by **{{.Labels.initiator}}**.'
WHERE
	id = '2faeee0f-26cb-4e96-821c-85ccb9f71513';
