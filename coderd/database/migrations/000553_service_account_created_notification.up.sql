-- Service accounts reuse the "user account created" notification, so the copy
-- must describe the account type that was actually created.
UPDATE notification_templates
SET
	title_template = E'{{ if eq (index .Labels "account_type") "service" }}Service{{ else }}User{{ end }} account "{{.Labels.created_account_name}}" created',
	body_template = E'New {{ if eq (index .Labels "account_type") "service" }}service{{ else }}user{{ end }} account **{{.Labels.created_account_name}}** has been created.\n\n' ||
					E'This new {{ if eq (index .Labels "account_type") "service" }}service{{ else }}user{{ end }} account was created {{if .Labels.created_account_user_name}}for **{{.Labels.created_account_user_name}}** {{end}}by **{{.Labels.initiator}}**.'
WHERE
	id = '4e19c0ac-94e1-4532-9515-d1801aa283b2';
