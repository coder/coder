UPDATE notification_templates
SET
	title_template = E'User account "{{.Labels.created_account_name}}" created',
	body_template = E'New user account **{{.Labels.created_account_name}}** has been created.\n\n' ||
					E'This new user account was created {{if .Labels.created_account_user_name}}for **{{.Labels.created_account_user_name}}** {{end}}by **{{.Labels.initiator}}**.'
WHERE
	id = '4e19c0ac-94e1-4532-9515-d1801aa283b2';
