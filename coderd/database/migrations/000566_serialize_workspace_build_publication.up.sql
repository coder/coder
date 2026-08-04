-- Publishing a new workspace build and acquiring a subagent execution both need
-- a stable answer to "which build is the workspace's latest generation". Under
-- READ COMMITTED each statement takes a fresh snapshot, so an acquisition that
-- validated the latest build could otherwise still commit after a newer build
-- became visible. A transaction-scoped advisory lock keyed on the workspace
-- serializes build publication against acquisition without adding contention
-- between unrelated workspaces.
CREATE FUNCTION acquire_workspace_build_publication_lock(workspace_id UUID) RETURNS void
	LANGUAGE plpgsql
	AS $$
BEGIN
	PERFORM pg_advisory_xact_lock(
		hashtext('workspace_build_publication'),
		hashtext(workspace_id::text)
	);
END;
$$;

COMMENT ON FUNCTION acquire_workspace_build_publication_lock(workspace_id UUID) IS
	'Takes the transaction-scoped advisory lock that serializes workspace build publication against readers that must observe the workspace''s latest build.';

CREATE FUNCTION serialize_workspace_build_publication() RETURNS trigger
	LANGUAGE plpgsql
	AS $$
BEGIN
	PERFORM acquire_workspace_build_publication_lock(NEW.workspace_id);
	RETURN NEW;
END;
$$;

COMMENT ON FUNCTION serialize_workspace_build_publication() IS
	'Trigger wrapper that takes the workspace build publication lock before a new workspace build row is inserted.';

CREATE TRIGGER serialize_workspace_build_publication
	BEFORE INSERT ON workspace_builds
	FOR EACH ROW
	EXECUTE FUNCTION serialize_workspace_build_publication();
