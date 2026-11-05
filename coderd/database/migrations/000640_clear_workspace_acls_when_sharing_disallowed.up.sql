-- Workspace ACL entries must not exist when the organization's sharing
-- setting does not allow the workspace owner's workspaces to be shared.
-- Changing the setting already deleted such entries, but entries could
-- survive or be written later (for example by older replicas or requests
-- that raced the setting change) and kept granting access. The triggers
-- below enforce the rule on every write, so all readers (RBAC objects, SQL
-- authorization filters, and the API) only ever see allowed entries.

CREATE FUNCTION workspace_sharing_allowed(organization_id uuid, owner_id uuid) RETURNS boolean
	LANGUAGE sql
	STABLE
	AS $$
	SELECT EXISTS (
		SELECT 1
		FROM organizations
		JOIN users ON users.id = workspace_sharing_allowed.owner_id
		WHERE organizations.id = workspace_sharing_allowed.organization_id
			AND (
				organizations.shareable_workspace_owners = 'everyone'
				OR (
					organizations.shareable_workspace_owners = 'service_accounts'
					AND users.is_service_account
				)
			)
	);
$$;

COMMENT ON FUNCTION workspace_sharing_allowed(uuid, uuid) IS 'Reports whether the organization sharing setting allows sharing workspaces owned by the user.';

CREATE FUNCTION clear_workspace_acls_when_sharing_disallowed() RETURNS trigger
	LANGUAGE plpgsql
	AS $$
BEGIN
	IF NOT workspace_sharing_allowed(NEW.organization_id, NEW.owner_id) THEN
		NEW.group_acl := '{}'::jsonb;
		NEW.user_acl := '{}'::jsonb;
	END IF;
	RETURN NEW;
END;
$$;

COMMENT ON FUNCTION clear_workspace_acls_when_sharing_disallowed() IS 'Clears the ACLs of a written workspace when its organization does not allow sharing it.';

CREATE TRIGGER trigger_clear_workspace_acls_when_sharing_disallowed
	BEFORE INSERT OR UPDATE OF group_acl, user_acl, owner_id, organization_id ON workspaces
	FOR EACH ROW
	WHEN (NEW.group_acl <> '{}'::jsonb OR NEW.user_acl <> '{}'::jsonb)
	EXECUTE FUNCTION clear_workspace_acls_when_sharing_disallowed();

CREATE FUNCTION clear_workspace_acls_on_sharing_setting_change() RETURNS trigger
	LANGUAGE plpgsql
	AS $$
BEGIN
	UPDATE workspaces
	SET
		group_acl = '{}'::jsonb,
		user_acl = '{}'::jsonb
	WHERE workspaces.organization_id = NEW.id
		AND (workspaces.group_acl <> '{}'::jsonb OR workspaces.user_acl <> '{}'::jsonb)
		AND NOT workspace_sharing_allowed(workspaces.organization_id, workspaces.owner_id);
	RETURN NULL;
END;
$$;

COMMENT ON FUNCTION clear_workspace_acls_on_sharing_setting_change() IS 'Clears the ACLs of workspaces that an organization no longer allows sharing after its sharing setting changes.';

CREATE TRIGGER trigger_clear_workspace_acls_on_sharing_setting_change
	AFTER UPDATE OF shareable_workspace_owners ON organizations
	FOR EACH ROW
	WHEN (NEW.shareable_workspace_owners IS DISTINCT FROM OLD.shareable_workspace_owners)
	EXECUTE FUNCTION clear_workspace_acls_on_sharing_setting_change();

-- Remove entries left behind before these triggers existed.
UPDATE workspaces
SET
	group_acl = '{}'::jsonb,
	user_acl = '{}'::jsonb
WHERE (group_acl <> '{}'::jsonb OR user_acl <> '{}'::jsonb)
	AND NOT workspace_sharing_allowed(organization_id, owner_id);
