UPDATE notification_templates SET body_template = E'Your workspace **{{.Labels.name}}** has been marked as [**dormant**](https://coder.com/docs/templates/schedule#dormancy-threshold-enterprise) due to inactivity exceeding the dormancy threshold.\n\n' ||
	E'This workspace will be automatically deleted in {{.Labels.timeTilDormant}} if it remains inactive.\n\n' ||
	E'To prevent deletion, activate your workspace using the link below.' WHERE id = '0ea69165-ec14-4314-91f1-69566ac3c5a0';

UPDATE notification_templates SET body_template = E'Your workspace **{{.Labels.name}}** has been marked for **deletion** after {{.Labels.timeTilDormant}} of [dormancy](https://coder.com/docs/templates/schedule#dormancy-auto-deletion-enterprise) because of {{.Labels.reason}}.\n' ||
	E'To prevent deletion, use your workspace with the link below.' WHERE id = '51ce2fdf-c9ca-4be1-8d70-628674f9bc42';
