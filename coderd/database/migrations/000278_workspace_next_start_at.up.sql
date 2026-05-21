ALTER TABLE ONLY workspaces ADD COLUMN IF NOT EXISTS next_start_at TIMESTAMPTZ DEFAULT NULL;

CREATE INDEX workspace_next_start_at_idx ON workspaces USING btree (next_start_at) WHERE (deleted=false);
CREATE INDEX workspace_template_id_idx   ON workspaces USING btree (template_id)   WHERE (deleted=false);

CREATE FUNCTION nullify_next_start_at_on_workspace_autostart_modification() RETURNS trigger
	LANGUAGE plpgsql
AS $$
DECLARE
BEGIN
	-- A workspace's next_start_at might be invalidated by the following:
	--   * The autostart schedule has changed independent to next_start_at
	--   * The workspace has been marked as dormant
	IF (NEW.autostart_schedule <> OLD.autostart_schedule AND NEW.next_start_at = OLD.next_start_at)
		OR (NEW.dormant_at IS NOT NULL AND NEW.next_start_at IS NOT NULL)
	THEN
		UPDATE workspaces
		SET next_start_at = NULL
		WHERE id = NEW.id;
	END IF;
	RETURN NEW;
END;
$$;

CREATE TRIGGER trigger_nullify_next_start_at_on_workspace_autostart_modification
	AFTER UPDATE ON workspaces
	FOR EACH ROW
EXECUTE PROCEDURE nullify_next_start_at_on_workspace_autostart_modification();

-- Recreate view
DROP VIEW workspaces_expanded;

CREATE VIEW
	workspaces_expanded
AS
SELECT
	workspaces.*,
	-- Owner
	visible_users.avatar_url AS owner_avatar_url,
	visible_users.username AS owner_username,
	-- Organization
	organizations.name AS organization_name,
	organizations.display_name AS organization_display_name,
	organizations.icon AS organization_icon,
	organizations.description AS organization_description,
    -- Template
	templates.name AS template_name,
	templates.display_name AS template_display_name,
	templates.icon AS template_icon,
	templates.description AS template_description
FROM
	workspaces
	INNER JOIN
		visible_users
	ON
		workspaces.owner_id = visible_users.id
	INNER JOIN
		organizations
	ON workspaces.organization_id = organizations.id
	INNER JOIN
		templates
	ON workspaces.template_id = templates.id
;

COMMENT ON VIEW workspaces_expanded IS 'Joins in the display name information such as username, avatar, and organization name.';
