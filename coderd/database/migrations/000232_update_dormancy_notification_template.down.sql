UPDATE notification_templates
SET
    body_template = E'Hi {{.UserName}}\n\n' || E'Your workspace **{{.Labels.name}}** has been marked as **dormant**.\n' || E'The specified reason was "**{{.Labels.reason}} (initiated by: {{ .Labels.initiator }})**\n\n' || E'Dormancy refers to a workspace being unused for a defined length of time, and after it exceeds {{.Labels.dormancyHours}} hours of dormancy might be deleted.\n' || E'To activate your workspace again, simply use it as normal.'
WHERE
    id = '0ea69165-ec14-4314-91f1-69566ac3c5a0';

UPDATE notification_templates
SET
    body_template = E'Hi {{.UserName}}\n\n' || E'Your workspace **{{.Labels.name}}** has been marked for **deletion** after {{.Labels.dormancyHours}} hours of dormancy.\n' || E'The specified reason was "**{{.Labels.reason}}**\n\n' || E'Dormancy refers to a workspace being unused for a defined length of time, and after it exceeds {{.Labels.dormancyHours}} hours of dormancy it will be deleted.\n' || E'To prevent your workspace from being deleted, simply use it as normal.'
WHERE
    id = '51ce2fdf-c9ca-4be1-8d70-628674f9bc42';
