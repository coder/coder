UPDATE notification_templates
SET
	body_template = REPLACE(body_template::text, 'Hi {{.UserName}}', 'Hi {{.UserName}},')::text
WHERE
	id = '29a09665-2a4c-403f-9648-54301670e7be';

UPDATE notification_templates
SET
	body_template = REPLACE(body_template::text, 'Hi {{.UserName}},', 'Hi {{.UserName}},\n')::text
WHERE
	id = '9f5af851-8408-4e73-a7a1-c6502ba46689';

UPDATE notification_templates
SET
	body_template = REPLACE(body_template::text, 'Hi {{.UserName}},', 'Hi {{.UserName}},\n')::text
WHERE
	id = 'b02ddd82-4733-4d02-a2d7-c36f3598997d';

UPDATE notification_templates
SET
	body_template = REPLACE(body_template::text, 'Hi {{.UserName}}', 'Hi {{.UserName}},\n')::text
WHERE
	id = 'c34a0c09-0704-4cac-bd1c-0c0146811c2b';

UPDATE notification_templates
SET
	body_template = REPLACE(body_template::text, '({{.Labels.template_version_name}}).\n', '({{.Labels.template_version_name}}).\n\n')::text
WHERE
	id = 'c34a0c09-0704-4cac-bd1c-0c0146811c2b';

UPDATE notification_templates
SET
	body_template = REPLACE(body_template::text, 'Hi {{.UserName}}', 'Hi {{.UserName}},\n')::text
WHERE
	id = '381df2a9-c0c0-4749-420f-80a9280c66f9';

UPDATE notification_templates
SET
	body_template = REPLACE(body_template::text, 'The specified reason', '\nThe specified reason')::text
WHERE
	id = '381df2a9-c0c0-4749-420f-80a9280c66f9';

UPDATE notification_templates
SET
	body_template = REPLACE(body_template::text, 'Hi {{.UserName}}', 'Hi {{.UserName}},')::text
WHERE
	id = 'f517da0b-cdc9-410f-ab89-a86107c420ed';

UPDATE notification_templates
SET
	body_template = REPLACE(body_template::text, 'The specified reason', '\nThe specified reason')::text
WHERE
	id = 'f517da0b-cdc9-410f-ab89-a86107c420ed';

UPDATE notification_templates
SET
	body_template = REPLACE(body_template::text,  'Hi {{.UserName}}', 'Hi {{.UserName}},')::text
WHERE
	id = '0ea69165-ec14-4314-91f1-69566ac3c5a0';

UPDATE notification_templates
SET
	body_template = REPLACE(body_template::text,  'Hi {{.UserName}}', 'Hi {{.UserName}},')::text
WHERE
	id = '51ce2fdf-c9ca-4be1-8d70-628674f9bc42';

UPDATE notification_templates
SET
	body_template = REPLACE(body_template::text,  'The workspace build was initiated by', '\nThe workspace build was initiated by')::text
WHERE
	id = '2faeee0f-26cb-4e96-821c-85ccb9f71513';

UPDATE notification_templates
SET
	body_template = REPLACE(body_template::text, 'Hi {{.UserName}},', 'Hi {{.UserName}},\n')::text
WHERE
	id = '1a6a6bea-ee0a-43e2-9e7c-eabdb53730e4';

UPDATE notification_templates
SET
	body_template = REPLACE(body_template::text, 'Hi {{.UserName}},', 'Hi {{.UserName}},\n')::text
WHERE
	id = '6a2f0609-9b69-4d36-a989-9f5925b6cbff';
