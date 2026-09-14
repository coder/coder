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
					E'Your account **{{.Labels.suspended_account_name}}** has been suspended by **{{.Labels.account_suspender_user_name}}**.'
WHERE
	id = '6a2f0609-9b69-4d36-a989-9f5925b6cbff';


-- UserAccountActivated
UPDATE notification_templates
SET
	body_template = E'Hi {{.UserName}},\n\n' || -- Add a \n
					E'User account **{{.Labels.activated_account_name}}** has been activated.\n\n' ||
					E'The newly activated account belongs to **{{.Labels.activated_account_user_name}}** and was activated by **{{.Labels.account_activator_user_name}}**.'
WHERE
	id = '9f5af851-8408-4e73-a7a1-c6502ba46689';

-- YourAccountActivated
UPDATE notification_templates
SET
	body_template = E'Hi {{.UserName}},\n\n' || -- Add a \n
					E'Your account **{{.Labels.activated_account_name}}** has been activated by **{{.Labels.account_activator_user_name}}**.'
WHERE
	id = '1a6a6bea-ee0a-43e2-9e7c-eabdb53730e4';
