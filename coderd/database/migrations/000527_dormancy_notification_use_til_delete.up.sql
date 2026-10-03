-- Update the dormant workspace notification body so that the deletion
-- countdown references a dedicated `timeTilDelete` label, and to only
-- include the deletion time if the templates' `time_til_dormant_autodelete`
-- is enabled.
UPDATE notification_templates SET body_template = E'Your workspace **{{.Labels.name}}** has been marked as [**dormant**](https://coder.com/docs/admin/templates/managing-templates/schedule#dormancy-threshold) due to inactivity exceeding the dormancy threshold.\n\n' ||
	E'{{ if .Labels.timeTilDelete -}}\n' ||
	E'This workspace will be automatically deleted in {{.Labels.timeTilDelete}} if it remains inactive.\n\n' ||
	E'To prevent deletion, activate your workspace using the link below.\n' ||
	E'{{- else -}}\n' ||
	E'Activate your workspace using the link below to resume working in it.\n' ||
	E'{{- end }}' WHERE id = '0ea69165-ec14-4314-91f1-69566ac3c5a0';
