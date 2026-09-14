INSERT INTO notification_templates
	(id, name, title_template, body_template, "group", actions)
VALUES (
	'a9d027b4-ac49-4fb1-9f6d-45af15f64e7a',
	'Workspace Out Of Memory',
	E'Your workspace "{{.Labels.workspace}}" is low on memory',
	E'Hi {{.UserName}},\n\n'||
		E'Your workspace **{{.Labels.workspace}}** has reached the memory usage threshold set at **{{.Labels.threshold}}**.',
	'Workspace Events',
	'[
		{
			"label": "View workspace",
			"url": "{{base_url}}/@{{.UserUsername}}/{{.Labels.workspace}}"
		}
	]'::jsonb
);

INSERT INTO notification_templates
	(id, name, title_template, body_template, "group", actions)
VALUES (
	'f047f6a3-5713-40f7-85aa-0394cce9fa3a',
	'Workspace Out Of Disk',
	E'Your workspace "{{.Labels.workspace}}" is low on volume space',
	E'Hi {{.UserName}},\n\n'||
		E'{{ if eq (len .Data.volumes) 1 }}{{ $volume := index .Data.volumes 0 }}'||
			E'Volume **`{{$volume.path}}`** is over {{$volume.threshold}} full in workspace **{{.Labels.workspace}}**.'||
		E'{{ else }}'||
			E'The following volumes are nearly full in workspace **{{.Labels.workspace}}**\n\n'||
			E'{{ range $volume := .Data.volumes }}'||
				E'- **`{{$volume.path}}`** is over {{$volume.threshold}} full\n'||
			E'{{ end }}'||
		E'{{ end }}',
	'Workspace Events',
	'[
		{
			"label": "View workspace",
			"url": "{{base_url}}/@{{.UserUsername}}/{{.Labels.workspace}}"
		}
	]'::jsonb
);
