UPDATE notification_templates
SET
	name = 'Report: Workspace Builds Failed',
	title_template = 'Failed workspace builds report',
	body_template =
E'The following templates have had build failures over the last {{.Data.report_frequency}}:
{{range $template := .Data.templates}}
- **{{$template.display_name}}** failed to build {{$template.failed_builds}}/{{$template.total_builds}} times
{{end}}

**Report:**
{{range $template := .Data.templates}}
**{{$template.display_name}}**
{{range $version := $template.versions}}
- **{{$version.template_version_name}}** failed {{$version.failed_count}} time{{if gt $version.failed_count 1.0}}s{{end}}:
{{range $build := $version.failed_builds}}
   - [{{$build.workspace_owner_username}} / {{$build.workspace_name}} / #{{$build.build_number}}]({{base_url}}/@{{$build.workspace_owner_username}}/{{$build.workspace_name}}/builds/{{$build.build_number}})
{{end}}
{{end}}
{{end}}

We recommend reviewing these issues to ensure future builds are successful.'
WHERE id = '34a20db2-e9cc-4a93-b0e4-8569699d7a00';
