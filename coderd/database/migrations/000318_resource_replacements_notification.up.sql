INSERT INTO notification_templates
	(id, name, title_template, body_template, "group", actions)
VALUES ('89d9745a-816e-4695-a17f-3d0a229e2b8d',
		'Prebuilt Workspace Resource Replaced',
		E'There might be a problem with a recently claimed prebuilt workspace',
		$$
Workspace **{{.Labels.workspace}}** was claimed from a prebuilt workspace by **{{.Labels.claimant}}**.
During the claim, Terraform destroyed and recreated the following resources
because one or more immutable attributes changed:

{{range $resource, $paths := .Data.replacements -}}
- _{{ $resource }}_  was replaced due to changes to _{{ $paths }}_
{{end}}

When Terraform must change an immutable attribute, it replaces the entire resource.
If you’re using prebuilds to speed up provisioning, unexpected replacements will slow down
workspace startup—even when claiming a prebuilt environment.

For tips on preventing replacements and improving claim performance, see [this guide](https://coder.com/docs/TODO).
$$,
		'Workspace Events',
		'[
		{
			"label": "View workspace build",
			"url": "{{base_url}}/@{{.Labels.claimant}}/{{.Labels.workspace}}/builds/{{.Labels.workspace_build_num}}"
		}
	]'::jsonb);
