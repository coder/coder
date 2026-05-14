-- name: InsertTemplateVersionDLPPolicy :one
INSERT INTO template_version_dlp_policies (
	id,
	template_version_id,
	name,
	ssh_access,
	web_terminal_access,
	port_forwarding_access,
	desktop_access,
	clipboard_access,
	allowed_applications,
	display_name,
	created_at
)
VALUES (
	@id,
	@template_version_id,
	@name,
	@ssh_access,
	@web_terminal_access,
	@port_forwarding_access,
	@desktop_access,
	@clipboard_access,
	@allowed_applications,
	@display_name,
	@created_at
) RETURNING *;

-- name: GetTemplateVersionDLPPolicyByTemplateVersionID :one
SELECT
	*
FROM
	template_version_dlp_policies
WHERE
	template_version_id = @template_version_id;

-- name: GetTemplateVersionDLPPolicyByWorkspaceID :one
SELECT
	template_version_dlp_policies.*
FROM
	template_version_dlp_policies
	INNER JOIN workspace_builds ON workspace_builds.template_version_id = template_version_dlp_policies.template_version_id
WHERE
	workspace_builds.workspace_id = @workspace_id
ORDER BY
	workspace_builds.build_number DESC
LIMIT 1;
