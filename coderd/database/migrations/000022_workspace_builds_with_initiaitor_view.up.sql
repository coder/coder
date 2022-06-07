-- This view adds the initiator name to the query for UI purposes.
-- Showing the initiator user ID is not very friendly.
CREATE VIEW workspace_build_with_initiator AS
-- If the user is nil, just use 'unknown' for now.
SELECT workspace_builds.*, coalesce(users.username, 'unknown') AS initiator_username
FROM workspace_builds
		 LEFT JOIN users ON workspace_builds.initiator_id = users.id;
