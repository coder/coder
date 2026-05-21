UPDATE notification_templates SET body_template = E'Your workspace **{{.Labels.name}}** has been marked as [**dormant**](https://coder.com/docs/templates/schedule#dormancy-threshold-enterprise) because of {{.Labels.reason}}.\n' ||
		E'Dormant workspaces are [automatically deleted](https://coder.com/docs/templates/schedule#dormancy-auto-deletion-enterprise) after {{.Labels.timeTilDormant}} of inactivity.\n' ||
		E'To prevent deletion, use your workspace with the link below.' WHERE id = '0ea69165-ec14-4314-91f1-69566ac3c5a0';
