UPDATE notification_templates
SET
    title_template = E'Reset your password for Coder',
    body_template = E'Hi {{.UserName}},\n\nUse the link below to reset your password.\n\nIf you did not make this request, you can ignore this message.',
    actions = '[{
		"label": "Reset password",
		"url": "{{base_url}}/reset-password/change?otp={{.Labels.one_time_passcode}}&email={{.UserEmail | urlquery}}"
	}]'::jsonb
WHERE
    id = '62f86a30-2330-4b61-a26d-311ff3b608cf'
