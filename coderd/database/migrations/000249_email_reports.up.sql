INSERT INTO notification_templates (id, name, title_template, body_template, "group", actions)
VALUES ('bc0d9b9c-6dca-40a7-a209-fb2681e3341a', 'Report: Workspace Builds Failed For Template', E'Workspace builds failed for template "{{.Labels.template_display_name}}"',
        E'Hi {{.UserName}},

Template {{.Labels.template_display_name}} has failed to build {{.Labels.failed_builds})/{{.Labels.total_builds}} times over the last {{.Labels.report_frequency}} and may be unstable.

**Report:**

{{range $index, $version := .Labels.template_versions}}
  {{add $index 1}}. "{{$version.TemplateDisplayName}}" failed {{$version.FailedCount}} time{{if gt $version.FailedCount 1}}s{{end}}:
  {{range $i, $build := $version.FailedBuilds}}
    * [{{$build.WorkspaceOwnerUsername}} / {{$build.WorkspaceName}} / #{{$build.BuildNumber}}]({{base_url}}/@{{$build.WorkspaceOwnerUsername}}/{{$build.WorkspaceName}}/builds/{{$build.BuildNumber}})
  {{end}}
{{end}}

We recommend reviewing these issues to ensure future builds are successful.',
        'Template Events', '[
        {
            "label": "View workspaces",
            "url": "{{ base_url }}/workspaces?filter=template%3A{{.Labels.template_name}}"
        }
    ]'::jsonb);
