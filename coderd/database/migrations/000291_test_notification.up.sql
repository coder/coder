INSERT INTO notification_templates
	(id, name, title_template, body_template, "group", actions)
VALUES (
	'c425f63e-716a-4bf4-ae24-78348f706c3f',
	'Test Notification',
	E'A test notification',
	E'Hi {{.UserName}},\n\n'||
		E'This is a test notification.',
	'Notification Events',
	'[]'::jsonb
);
