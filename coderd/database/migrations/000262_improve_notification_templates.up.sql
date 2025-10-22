-- https://github.com/coder/coder/issues/14893

-- UserAccountSuspended
UPDATE notification_templates
SET
	body_template = E'Hi {{.UserName}},\n\n' || -- Add a \n
					E'User account **{{.Labels.suspended_account_name}}** has been suspended.\n\n' ||
					-- Mention the real name of the user who suspended the account:
  					E'The newly suspended account belongs to **{{.Labels.suspended_account_user_name}}** and was suspended by **{{.Labels.account_suspender_user_name}}**.'
WHERE
	id = 'b02ddd82-4733-4d02-a2d7-c36f3598997d';

-- YourAccountSuspended
UPDATE notification_templates
SET
	body_template = E'Hi {{.UserName}},\n\n' || -- Add a \n
					-- Mention who suspended the account:
					E'Your account **{{.Labels.suspended_account_name}}** has been suspended by **{{.Labels.account_suspender_user_name}}**.'
WHERE
	id = '6a2f0609-9b69-4d36-a989-9f5925b6cbff';

-- UserAccountActivated
UPDATE notification_templates
SET
	body_template = E'Hi {{.UserName}},\n\n' || -- Add a \n
					E'User account **{{.Labels.activated_account_name}}** has been activated.\n\n' ||
					-- Mention the real name of the user who activated the account:
					E'The newly activated account belongs to **{{.Labels.activated_account_user_name}}** and was activated by **{{.Labels.account_activator_user_name}}**.'
WHERE
	id = '9f5af851-8408-4e73-a7a1-c6502ba46689';

-- YourAccountActivated
UPDATE notification_templates
SET
	body_template = E'Hi {{.UserName}},\n\n' || -- Add a \n
					-- Mention who activated the account:
					E'Your account **{{.Labels.activated_account_name}}** has been activated by **{{.Labels.account_activator_user_name}}**.'
WHERE
	id = '1a6a6bea-ee0a-43e2-9e7c-eabdb53730e4';

-- UserAccountCreated
UPDATE notification_templates
SET
	body_template = E'Hi {{.UserName}},\n\n' ||
					E'New user account **{{.Labels.created_account_name}}** has been created.\n\n' ||
					-- Mention the real name of the user who created the account:
					E'This new user account was created for **{{.Labels.created_account_user_name}}** by **{{.Labels.account_creator}}**.'
WHERE
	id = '4e19c0ac-94e1-4532-9515-d1801aa283b2';

-- UserAccountDeleted
UPDATE notification_templates
SET
	body_template = E'Hi {{.UserName}},\n\n' ||
					E'User account **{{.Labels.deleted_account_name}}** has been deleted.\n\n' ||
					-- Mention the real name of the user who deleted the account:
					E'The deleted account belonged to **{{.Labels.deleted_account_user_name}}** and was deleted by **{{.Labels.account_deleter_user_name}}**.'
WHERE
	id = 'f44d9314-ad03-4bc8-95d0-5cad491da6b6';

-- TemplateDeleted
UPDATE notification_templates
SET
	body_template = E'Hi {{.UserName}},\n\n' || -- Add a comma
					E'The template **{{.Labels.name}}** was deleted by **{{ .Labels.initiator }}**.\n\n' ||
					-- Mention template display name:
					E'The template''s display name was **{{.Labels.display_name}}**.'
WHERE
	id = '29a09665-2a4c-403f-9648-54301670e7be';

-- WorkspaceAutoUpdated
UPDATE notification_templates
SET body_template = E'Hi {{.UserName}},\n\n' || -- Add a comma and a \n
					-- Add a \n:
					E'Your workspace **{{.Labels.name}}** has been updated automatically to the latest template version ({{.Labels.template_version_name}}).\n\n' ||
					E'Reason for update: **{{.Labels.template_version_message}}**.'
WHERE
	id = 'c34a0c09-0704-4cac-bd1c-0c0146811c2b';

-- WorkspaceAutoBuildFailed
UPDATE notification_templates
SET
	body_template = E'Hi {{.UserName}},\n\n' || -- Add a comma
					-- Add a \n after:
					E'Automatic build of your workspace **{{.Labels.name}}** failed.\n\n' ||
					E'The specified reason was "**{{.Labels.reason}}**".'
WHERE
	id = '381df2a9-c0c0-4749-420f-80a9280c66f9';

-- WorkspaceDeleted
UPDATE notification_templates
SET
	body_template = E'Hi {{.UserName}},\n\n' || -- Add a comma
					-- Add a \n after:
					E'Your workspace **{{.Labels.name}}** was deleted.\n\n' ||
					E'The specified reason was "**{{.Labels.reason}}{{ if .Labels.initiator }} ({{ .Labels.initiator }}){{end}}**".'
WHERE
	id = 'f517da0b-cdc9-410f-ab89-a86107c420ed';

-- WorkspaceDormant
UPDATE notification_templates
SET
	body_template = E'Hi {{.UserName}},\n\n' || -- add comma
		E'Your workspace **{{.Labels.name}}** has been marked as [**dormant**](https://coder.com/docs/templates/schedule#dormancy-threshold-enterprise) because of {{.Labels.reason}}.\n' ||
		E'Dormant workspaces are [automatically deleted](https://coder.com/docs/templates/schedule#dormancy-auto-deletion-enterprise) after {{.Labels.timeTilDormant}} of inactivity.\n' ||
		E'To prevent deletion, use your workspace with the link below.'
WHERE
	id = '0ea69165-ec14-4314-91f1-69566ac3c5a0';

-- WorkspaceMarkedForDeletion
UPDATE notification_templates
SET
	body_template = E'Hi {{.UserName}},\n\n' || -- add comma
		E'Your workspace **{{.Labels.name}}** has been marked for **deletion** after {{.Labels.timeTilDormant}} of [dormancy](https://coder.com/docs/templates/schedule#dormancy-auto-deletion-enterprise) because of {{.Labels.reason}}.\n' ||
		E'To prevent deletion, use your workspace with the link below.'
WHERE
	id = '51ce2fdf-c9ca-4be1-8d70-628674f9bc42';

-- WorkspaceManualBuildFailed
UPDATE notification_templates
SET
	body_template = E'Hi {{.UserName}},\n\n' ||
					E'A manual build of the workspace **{{.Labels.name}}** using the template **{{.Labels.template_name}}** failed (version: **{{.Labels.template_version_name}}**).\n\n' ||
					-- Mention template display name:
					E'The template''s display name was **{{.Labels.template_display_name}}**. ' ||
					E'The workspace build was initiated by **{{.Labels.initiator}}**.'
WHERE
	id = '2faeee0f-26cb-4e96-821c-85ccb9f71513';
