INSERT INTO notification_templates
(id, name, title_template, body_template, "group", actions)
VALUES ('414d9331-c1fc-4761-b40c-d1f4702279eb',
		'Prebuild Failure Limit Reached',
		E'There is a problem creating prebuilt workspaces for the preset',
		$$
The number of failed prebuilds has reached the hard limit for template **{{ .Labels.template }}** and preset **{{ .Labels.preset }}**.

To resume prebuilds, fix the underlying issue and upload a new template version.
$$,
		'Template Events',
		'[
		{
			"label": "View failed workspaces",
			"url": "{{base_url}}/workspaces?filter=owner:prebuilds+status:failed+template:{{.Labels.template}}"
		},
		{
			"label": "View template version",
			"url": "{{base_url}}/templates/{{.Labels.org}}/{{.Labels.template}}/versions/{{.Labels.template_version}}"
		}
	]'::jsonb);
