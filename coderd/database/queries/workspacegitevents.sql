-- name: InsertWorkspaceGitEvent :one
INSERT INTO workspace_git_events (
	workspace_id,
	agent_id,
	owner_id,
	organization_id,
	event_type,
	session_id,
	commit_sha,
	commit_message,
	branch,
	repo_name,
	files_changed,
	agent_name,
	ai_bridge_interception_id
) VALUES (
	@workspace_id,
	@agent_id,
	@owner_id,
	@organization_id,
	@event_type,
	@session_id,
	@commit_sha,
	@commit_message,
	@branch,
	@repo_name,
	@files_changed,
	@agent_name,
	@ai_bridge_interception_id
)
RETURNING *;

-- name: GetWorkspaceGitEventByID :one
SELECT
	*
FROM
	workspace_git_events
WHERE
	id = @id::uuid;

-- name: ListWorkspaceGitEvents :many
SELECT
	*
FROM
	workspace_git_events wge
WHERE
	CASE
		WHEN @owner_id::uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN wge.owner_id = @owner_id::uuid
		ELSE true
	END
	AND CASE
		WHEN @workspace_id::uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN wge.workspace_id = @workspace_id::uuid
		ELSE true
	END
	AND CASE
		WHEN @organization_id::uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN wge.organization_id = @organization_id::uuid
		ELSE true
	END
	AND CASE
		WHEN @event_type::text != '' THEN wge.event_type = @event_type::text
		ELSE true
	END
	AND CASE
		WHEN @session_id::text != '' THEN wge.session_id = @session_id::text
		ELSE true
	END
	AND CASE
		WHEN @agent_name::text != '' THEN wge.agent_name = @agent_name::text
		ELSE true
	END
	AND CASE
		WHEN @repo_name::text != '' THEN wge.repo_name = @repo_name::text
		ELSE true
	END
	AND CASE
		WHEN @since::timestamptz != '0001-01-01 00:00:00+00'::timestamptz THEN wge.created_at >= @since::timestamptz
		ELSE true
	END
	AND CASE
		WHEN @after_id::uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN (
			(wge.created_at, wge.id) < (
				@after_created_at::timestamptz,
				@after_id::uuid
			)
		)
		ELSE true
	END
ORDER BY
	wge.created_at DESC,
	wge.id DESC
LIMIT COALESCE(NULLIF(@limit_opt::int, 0), 100);

-- name: CountWorkspaceGitEvents :one
SELECT
	COUNT(*) AS count
FROM
	workspace_git_events wge
WHERE
	CASE
		WHEN @owner_id::uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN wge.owner_id = @owner_id::uuid
		ELSE true
	END
	AND CASE
		WHEN @workspace_id::uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN wge.workspace_id = @workspace_id::uuid
		ELSE true
	END
	AND CASE
		WHEN @organization_id::uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN wge.organization_id = @organization_id::uuid
		ELSE true
	END
	AND CASE
		WHEN @event_type::text != '' THEN wge.event_type = @event_type::text
		ELSE true
	END
	AND CASE
		WHEN @session_id::text != '' THEN wge.session_id = @session_id::text
		ELSE true
	END
	AND CASE
		WHEN @agent_name::text != '' THEN wge.agent_name = @agent_name::text
		ELSE true
	END
	AND CASE
		WHEN @repo_name::text != '' THEN wge.repo_name = @repo_name::text
		ELSE true
	END
	AND CASE
		WHEN @since::timestamptz != '0001-01-01 00:00:00+00'::timestamptz THEN wge.created_at >= @since::timestamptz
		ELSE true
	END;

-- name: ListWorkspaceGitEventSessions :many
SELECT
	wge.session_id,
	wge.owner_id,
	wge.workspace_id,
	wge.agent_name,
	MIN(wge.created_at) AS started_at,
	MAX(wge.created_at) FILTER (WHERE wge.event_type = 'session_end') AS ended_at,
	COUNT(*) FILTER (WHERE wge.event_type = 'commit') AS commit_count,
	COUNT(*) FILTER (WHERE wge.event_type = 'push') AS push_count
FROM
	workspace_git_events wge
WHERE
	wge.session_id IS NOT NULL
	AND CASE
		WHEN @owner_id::uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN wge.owner_id = @owner_id::uuid
		ELSE true
	END
	AND CASE
		WHEN @workspace_id::uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN wge.workspace_id = @workspace_id::uuid
		ELSE true
	END
	AND CASE
		WHEN @organization_id::uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN wge.organization_id = @organization_id::uuid
		ELSE true
	END
	AND CASE
		WHEN @agent_name::text != '' THEN wge.agent_name = @agent_name::text
		ELSE true
	END
	AND CASE
		WHEN @repo_name::text != '' THEN wge.repo_name = @repo_name::text
		ELSE true
	END
	AND CASE
		WHEN @since::timestamptz != '0001-01-01 00:00:00+00'::timestamptz THEN wge.created_at >= @since::timestamptz
		ELSE true
	END
GROUP BY
	wge.session_id,
	wge.owner_id,
	wge.workspace_id,
	wge.agent_name
HAVING
	CASE
		WHEN @after_started_at::timestamptz != '0001-01-01 00:00:00+00'::timestamptz THEN MIN(wge.created_at) < @after_started_at::timestamptz
		ELSE true
	END
ORDER BY
	started_at DESC
LIMIT COALESCE(NULLIF(@limit_opt::int, 0), 50);

-- name: GetWorkspaceGitEventsBySessionID :many
SELECT
	*
FROM
	workspace_git_events
WHERE
	session_id = @session_id::text
ORDER BY
	created_at ASC;

-- name: DeleteOldWorkspaceGitEvents :one
WITH deleted AS (
	DELETE FROM workspace_git_events
	WHERE created_at < @before::timestamptz
	RETURNING 1
)
SELECT
	COUNT(*) AS count
FROM
	deleted;
