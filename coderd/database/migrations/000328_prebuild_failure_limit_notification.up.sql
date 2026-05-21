INSERT INTO notification_templates
(id, name, title_template, body_template, "group", actions)
VALUES ('414d9331-c1fc-4761-b40c-d1f4702279eb',
		'Prebuild Failure Limit Reached',
		E'There is a problem creating prebuilt workspaces',
		$$
The number of failed prebuild attempts has reached the hard limit for template **{{ .Labels.template }}** and preset **{{ .Labels.preset }}**.

To resume prebuilds, fix the underlying issue and upload a new template version.

Refer to the documentation for more details:
- [Troubleshooting templates](https://coder.com/docs/admin/templates/troubleshooting)
- [Troubleshooting of prebuilt workspaces](https://coder.com/docs/admin/templates/extending-templates/prebuilt-workspaces#administration-and-troubleshooting)
$$,
		'Template Events',
		'[
		{
			"label": "View failed prebuilt workspaces",
			"url": "{{base_url}}/workspaces?filter=owner:prebuilds+status:failed+template:{{.Labels.template}}"
		},
		{
			"label": "View template version",
			"url": "{{base_url}}/templates/{{.Labels.org}}/{{.Labels.template}}/versions/{{.Labels.template_version}}"
		}
	]'::jsonb);
