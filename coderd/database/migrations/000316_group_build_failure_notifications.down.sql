UPDATE notification_templates
SET
	name = 'Report: Workspace Builds Failed For Template',
	title_template = E'Workspace builds failed for template "{{.Labels.template_display_name}}"',
    body_template = E'Template **{{.Labels.template_display_name}}** has failed to build {{.Data.failed_builds}}/{{.Data.total_builds}} times over the last {{.Data.report_frequency}}.

**Report:**
{{range $version := .Data.template_versions}}
**{{$version.template_version_name}}** failed {{$version.failed_count}} time{{if gt $version.failed_count 1.0}}s{{end}}:
{{range $build := $version.failed_builds}}
* [{{$build.workspace_owner_username}} / {{$build.workspace_name}} / #{{$build.build_number}}]({{base_url}}/@{{$build.workspace_owner_username}}/{{$build.workspace_name}}/builds/{{$build.build_number}})
{{- end}}
{{end}}
We recommend reviewing these issues to ensure future builds are successful.',
        actions = '[
        {
            "label": "View workspaces",
            "url": "{{ base_url }}/workspaces?filter=template%3A{{.Labels.template_name}}"
        }
    ]'::jsonb)
WHERE id = '34a20db2-e9cc-4a93-b0e4-8569699d7a00';
