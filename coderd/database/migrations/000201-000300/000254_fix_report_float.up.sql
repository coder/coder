UPDATE notification_templates
SET
    body_template = REPLACE(body_template::text, '{{if gt $version.failed_count 1}}', '{{if gt $version.failed_count 1.0}}')::text
WHERE
    id = '34a20db2-e9cc-4a93-b0e4-8569699d7a00';
