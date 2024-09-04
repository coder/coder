INSERT INTO notification_templates (id, name, title_template, body_template, "group", actions)
VALUES ('bc0d9b9c-6dca-40a7-a209-fb2681e3341a', 'Report: Workspace Builds Failed For Template', E'Workspace builds failed for template "{{.Labels.template_display_name}}"',
        E'Hi {{.UserName}},

Template {{.Labels.template_display_name}} has failed to build {{.Data.failed_builds}}/{{.Data.total_builds}} times over the last {{.Data.report_frequency}} and may be unstable.

**Report:**

{{range $version := .Data.template_versions}}
  **{{$version.template_version_name}}** failed {{$version.failed_count}} time{{if gt $version.failed_count 1}}s{{end}}:
  {{range $build := $version.failed_builds}}
    * [{{$build.workspace_owner_username}} / {{$build.workspace_name}} / #{{$build.build_number}}]({{base_url}}/@{{$build.workspace_owner_username}}/{{$build.workspace_name}}/builds/{{$build.build_number}})
  {{end}}
{{end}}

We recommend reviewing these issues to ensure future builds are successful.',
        'Template Events', '[
        {
            "label": "View workspaces",
            "url": "{{ base_url }}/workspaces?filter=template%3A{{.Labels.template_name}}"
        }
    ]'::jsonb);
