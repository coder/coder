-- Default to `false`. Users will have to manually opt into the terraform workspace cache feature.
ALTER TABLE templates ADD COLUMN use_terraform_workspace_cache BOOL NOT NULL DEFAULT false;

COMMENT ON COLUMN templates.use_terraform_workspace_cache IS
	'Determines whether to keep terraform directories cached between runs for workspaces created from this template. '
		'When enabled, this can significantly speed up the `terraform init` step at the cost of increased disk usage. '
		'This is an opt-in experience, as it prevents modules from being updated, and therefore is a behavioral difference '
		'from the default.';
	;

-- Update the template_with_names view by recreating it.
DROP VIEW template_with_names;
CREATE VIEW template_with_names AS
SELECT
	templates.*,
   COALESCE(visible_users.avatar_url, ''::text) AS created_by_avatar_url,
   COALESCE(visible_users.username, ''::text) AS created_by_username,
   COALESCE(visible_users.name, ''::text) AS created_by_name,
   COALESCE(organizations.name, ''::text) AS organization_name,
   COALESCE(organizations.display_name, ''::text) AS organization_display_name,
   COALESCE(organizations.icon, ''::text) AS organization_icon
FROM
	templates
		LEFT JOIN
	visible_users
	ON
		templates.created_by = visible_users.id
		LEFT JOIN
	organizations
	ON templates.organization_id = organizations.id
;

COMMENT ON VIEW template_with_names IS 'Joins in the display name information such as username, avatar, and organization name.';
