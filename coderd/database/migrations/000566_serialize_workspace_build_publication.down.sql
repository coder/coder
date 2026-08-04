DROP TRIGGER serialize_workspace_build_publication ON workspace_builds;

DROP FUNCTION serialize_workspace_build_publication();

DROP FUNCTION acquire_workspace_build_publication_lock(workspace_id UUID);
