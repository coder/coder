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

-- name: GetTemplateVersionDLPPoliciesByTemplateVersionID :many
SELECT
	*
FROM
	template_version_dlp_policies
WHERE
	template_version_id = @template_version_id;

-- name: GetTemplateVersionDLPPolicyByVersionAndName :one
SELECT
	*
FROM
	template_version_dlp_policies
WHERE
	template_version_id = @template_version_id
	AND name = @name;

-- name: GetTemplateVersionDLPPolicyByAgentID :one
SELECT
	template_version_dlp_policies.*
FROM
	template_version_dlp_policies
	INNER JOIN workspace_agents ON workspace_agents.dlp_policy_id = template_version_dlp_policies.id
WHERE
	workspace_agents.id = @agent_id;
