-- Default to `false`. Users will have to manually opt back into the classic parameter flow.
-- We want the new experience to be tried first.
ALTER TABLE templates ADD COLUMN use_classic_parameter_flow BOOL NOT NULL DEFAULT false;

COMMENT ON COLUMN templates.use_classic_parameter_flow IS
	'Determines whether to default to the dynamic parameter creation flow for this template '
	'or continue using the legacy classic parameter creation flow.'
	'This is a template wide setting, the template admin can revert to the classic flow if there are any issues. '
	'An escape hatch is required, as workspace creation is a core workflow and cannot break. '
	'This column will be removed when the dynamic parameter creation flow is stable.';


-- Update the template_with_names view by recreating it.
DROP VIEW template_with_names;
CREATE VIEW
	template_with_names
AS
SELECT
	templates.*,
	coalesce(visible_users.avatar_url, '') AS created_by_avatar_url,
	coalesce(visible_users.username, '') AS created_by_username,
	coalesce(organizations.name, '') AS organization_name,
	coalesce(organizations.display_name, '') AS organization_display_name,
	coalesce(organizations.icon, '') AS organization_icon
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
