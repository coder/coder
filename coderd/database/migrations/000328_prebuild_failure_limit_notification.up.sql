INSERT INTO notification_templates
(id, name, title_template, body_template, "group", actions)
VALUES ('414d9331-c1fc-4761-b40c-d1f4702279eb',
		'Prebuild Failure Limit Reached',
		E'There is a problem creating prebuilt workspaces for the preset',
		$$
			The number of failed prebuilds has reached the hard limit for template **{{ .Labels.template }}** and preset **{{ .Labels.preset }}**
		$$,
		'Template Events',
		'[]'::jsonb);
