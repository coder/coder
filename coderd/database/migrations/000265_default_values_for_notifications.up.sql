
-- https://github.com/coder/coder/issues/14893

-- UserAccountSuspended
UPDATE notification_templates
SET
	body_template = E'Hi {{.UserName}},\n\n' ||
					E'User account **{{.Labels.suspended_account_name}}** has been suspended.\n\n' ||
  					E'The account {{if .Labels.suspended_account_user_name}}belongs to **{{.Labels.suspended_account_user_name}}** and it {{end}}was suspended by **{{.Labels.initiator}}**.'

WHERE
	id = 'b02ddd82-4733-4d02-a2d7-c36f3598997d';

-- UserAccountActivated
UPDATE notification_templates
SET
	body_template = E'Hi {{.UserName}},\n\n' || -- Add a \n
					E'User account **{{.Labels.activated_account_name}}** has been activated.\n\n' ||
					E'The account {{if .Labels.activated_account_user_name}}belongs to **{{.Labels.activated_account_user_name}}** and it {{ end }}was activated by **{{.Labels.initiator}}**.'
WHERE
	id = '9f5af851-8408-4e73-a7a1-c6502ba46689';

-- UserAccountCreated
UPDATE notification_templates
SET
	body_template = E'Hi {{.UserName}},\n\n' ||
					E'New user account **{{.Labels.created_account_name}}** has been created.\n\n' ||
					E'This new user account was created {{if .Labels.created_account_user_name}}for **{{.Labels.created_account_user_name}}** {{end}}by **{{.Labels.initiator}}**.'
WHERE
	id = '4e19c0ac-94e1-4532-9515-d1801aa283b2';

-- UserAccountDeleted
UPDATE notification_templates
SET
	body_template = E'Hi {{.UserName}},\n\n' ||
					E'User account **{{.Labels.deleted_account_name}}** has been deleted.\n\n' ||
					E'The deleted account {{if .Labels.deleted_account_user_name}}belonged to **{{.Labels.deleted_account_user_name}}** and {{end}}was deleted by **{{.Labels.initiator}}**.'
WHERE
	id = 'f44d9314-ad03-4bc8-95d0-5cad491da6b6';
