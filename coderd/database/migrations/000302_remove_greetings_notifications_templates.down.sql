UPDATE notification_templates SET body_template = E'Hi {{.UserName}},\n\n' ||
					E'Your workspace **{{.Labels.name}}** was deleted.\n\n' ||
					E'The specified reason was "**{{.Labels.reason}}{{ if .Labels.initiator }} ({{ .Labels.initiator }}){{end}}**".' WHERE id = 'f517da0b-cdc9-410f-ab89-a86107c420ed';
UPDATE notification_templates SET body_template = E'Hi {{.UserName}},\n\n' ||
					E'Automatic build of your workspace **{{.Labels.name}}** failed.\n\n' ||
					E'The specified reason was "**{{.Labels.reason}}**".' WHERE id = '381df2a9-c0c0-4749-420f-80a9280c66f9';
UPDATE notification_templates SET body_template = E'Hi {{.UserName}},\n\n' ||
					E'Your workspace **{{.Labels.name}}** has been updated automatically to the latest template version ({{.Labels.template_version_name}}).\n\n' ||
					E'Reason for update: **{{.Labels.template_version_message}}**.' WHERE id = 'c34a0c09-0704-4cac-bd1c-0c0146811c2b';
UPDATE notification_templates SET body_template = E'Hi {{.UserName}},\n\n' ||
					E'New user account **{{.Labels.created_account_name}}** has been created.\n\n' ||
					E'This new user account was created {{if .Labels.created_account_user_name}}for **{{.Labels.created_account_user_name}}** {{end}}by **{{.Labels.initiator}}**.' WHERE id = '4e19c0ac-94e1-4532-9515-d1801aa283b2';
UPDATE notification_templates SET body_template = E'Hi {{.UserName}},\n\n' ||
					E'User account **{{.Labels.deleted_account_name}}** has been deleted.\n\n' ||
					E'The deleted account {{if .Labels.deleted_account_user_name}}belonged to **{{.Labels.deleted_account_user_name}}** and {{end}}was deleted by **{{.Labels.initiator}}**.' WHERE id = 'f44d9314-ad03-4bc8-95d0-5cad491da6b6';
UPDATE notification_templates SET body_template = E'Hi {{.UserName}},\n\n' ||
					E'User account **{{.Labels.suspended_account_name}}** has been suspended.\n\n' ||
  					E'The account {{if .Labels.suspended_account_user_name}}belongs to **{{.Labels.suspended_account_user_name}}** and it {{end}}was suspended by **{{.Labels.initiator}}**.' WHERE id = 'b02ddd82-4733-4d02-a2d7-c36f3598997d';
UPDATE notification_templates SET body_template = E'Hi {{.UserName}},\nYour account **{{.Labels.suspended_account_name}}** has been suspended.' WHERE id = '6a2f0609-9b69-4d36-a989-9f5925b6cbff';
UPDATE notification_templates SET body_template = E'Hi {{.UserName}},\n\n' ||
					E'User account **{{.Labels.activated_account_name}}** has been activated.\n\n' ||
					E'The account {{if .Labels.activated_account_user_name}}belongs to **{{.Labels.activated_account_user_name}}** and it {{ end }}was activated by **{{.Labels.initiator}}**.' WHERE id = '9f5af851-8408-4e73-a7a1-c6502ba46689';
UPDATE notification_templates SET body_template = E'Hi {{.UserName}},\n\n' ||
					E'Your account **{{.Labels.activated_account_name}}** has been activated by **{{.Labels.initiator}}**.' WHERE id = '1a6a6bea-ee0a-43e2-9e7c-eabdb53730e4';
UPDATE notification_templates SET body_template = E'Hi {{.UserName}},\n\nA manual build of the workspace **{{.Labels.name}}** using the template **{{.Labels.template_name}}** failed (version: **{{.Labels.template_version_name}}**).\nThe workspace build was initiated by **{{.Labels.initiator}}**.' WHERE id = '2faeee0f-26cb-4e96-821c-85ccb9f71513';
UPDATE notification_templates SET body_template = E'Hi {{.UserName}},

Template **{{.Labels.template_display_name}}** has failed to build {{.Data.failed_builds}}/{{.Data.total_builds}} times over the last {{.Data.report_frequency}}.

**Report:**
{{range $version := .Data.template_versions}}
**{{$version.template_version_name}}** failed {{$version.failed_count}} time{{if gt $version.failed_count 1.0}}s{{end}}:
{{range $build := $version.failed_builds}}
* [{{$build.workspace_owner_username}} / {{$build.workspace_name}} / #{{$build.build_number}}]({{base_url}}/@{{$build.workspace_owner_username}}/{{$build.workspace_name}}/builds/{{$build.build_number}})
{{- end}}
{{end}}
We recommend reviewing these issues to ensure future builds are successful.' WHERE id = '34a20db2-e9cc-4a93-b0e4-8569699d7a00';
UPDATE notification_templates SET body_template = E'Hi {{.UserName}},\n\nUse the link below to reset your password.\n\nIf you did not make this request, you can ignore this message.' WHERE id = '62f86a30-2330-4b61-a26d-311ff3b608cf';
UPDATE notification_templates SET body_template = E'Hello {{.UserName}},\n\n'||
		E'The template **{{.Labels.template}}** has been deprecated with the following message:\n\n' ||
		E'**{{.Labels.message}}**\n\n' ||
		E'New workspaces may not be created from this template. Existing workspaces will continue to function normally.' WHERE id = 'f40fae84-55a2-42cd-99fa-b41c1ca64894';
UPDATE notification_templates SET body_template = E'Hello {{.UserName}},\n\n'||
		E'The workspace **{{.Labels.workspace}}** has been created from the template **{{.Labels.template}}** using version **{{.Labels.version}}**.' WHERE id = '281fdf73-c6d6-4cbb-8ff5-888baf8a2fff';
UPDATE notification_templates SET body_template = E'Hello {{.UserName}},\n\n'||
		E'A new workspace build has been manually created for your workspace **{{.Labels.workspace}}** by **{{.Labels.initiator}}** to update it to version **{{.Labels.version}}** of template **{{.Labels.template}}**.' WHERE id = 'd089fe7b-d5c5-4c0c-aaf5-689859f7d392';
UPDATE notification_templates SET body_template = E'Hi {{.UserName}},\n\n'||
		E'Your workspace **{{.Labels.workspace}}** has reached the memory usage threshold set at **{{.Labels.threshold}}**.' WHERE id = 'a9d027b4-ac49-4fb1-9f6d-45af15f64e7a';
UPDATE notification_templates SET body_template = E'Hi {{.UserName}},\n\n'||
		E'{{ if eq (len .Data.volumes) 1 }}{{ $volume := index .Data.volumes 0 }}'||
			E'Volume **`{{$volume.path}}`** is over {{$volume.threshold}} full in workspace **{{.Labels.workspace}}**.'||
		E'{{ else }}'||
			E'The following volumes are nearly full in workspace **{{.Labels.workspace}}**\n\n'||
			E'{{ range $volume := .Data.volumes }}'||
				E'- **`{{$volume.path}}`** is over {{$volume.threshold}} full\n'||
			E'{{ end }}'||
		E'{{ end }}' WHERE id = 'f047f6a3-5713-40f7-85aa-0394cce9fa3a';
UPDATE notification_templates SET body_template = E'Hi {{.UserName}},\n\n'||
		E'This is a test notification.' WHERE id = 'c425f63e-716a-4bf4-ae24-78348f706c3f';
