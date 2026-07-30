UPDATE notification_templates
SET
    body_template = E'Hi {{.UserName}},\n\nNew user account **{{.Labels.created_account_name}}** has been created.'
WHERE
    id = '4e19c0ac-94e1-4532-9515-d1801aa283b2';
