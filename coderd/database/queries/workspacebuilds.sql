-- name: GetWorkspaceBuildByID :one
SELECT
	*
FROM
	workspace_build_with_user AS workspace_builds
WHERE
	id = $1
LIMIT
	1;

-- name: GetWorkspaceBuildByJobID :one
SELECT
	*
FROM
	workspace_build_with_user AS workspace_builds
WHERE
	job_id = $1
LIMIT
	1;

-- name: GetWorkspaceBuildsCreatedAfter :many
SELECT * FROM workspace_build_with_user WHERE created_at > $1;

-- name: GetWorkspaceBuildByWorkspaceIDAndBuildNumber :one
SELECT
	*
FROM
	workspace_build_with_user AS workspace_builds
WHERE
	workspace_id = $1
	AND build_number = $2;

-- name: GetWorkspaceBuildsByWorkspaceID :many
SELECT
	*
FROM
	workspace_build_with_user AS workspace_builds
WHERE
	workspace_builds.workspace_id = $1
	AND workspace_builds.created_at > @since
    AND CASE
		-- This allows using the last element on a page as effectively a cursor.
		-- This is an important option for scripts that need to paginate without
		-- duplicating or missing data.
		WHEN @after_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN (
			-- The pagination cursor is the last ID of the previous page.
			-- The query is ordered by the build_number field, so select all
			-- rows after the cursor.
			build_number > (
				SELECT
					build_number
				FROM
					workspace_builds
				WHERE
					id = @after_id
			)
		)
		ELSE true
END
ORDER BY
    build_number desc OFFSET @offset_opt
LIMIT
    -- A null limit means "no limit", so 0 means return all
    NULLIF(@limit_opt :: int, 0);

-- name: GetLatestWorkspaceBuildByWorkspaceID :one
SELECT
	*
FROM
	workspace_build_with_user AS workspace_builds
WHERE
	workspace_id = $1
ORDER BY
    build_number desc
LIMIT
	1;

-- name: GetLatestWorkspaceBuildsByWorkspaceIDs :many
SELECT wb.*
FROM (
    SELECT
        workspace_id, MAX(build_number) as max_build_number
    FROM
		workspace_build_with_user AS workspace_builds
    WHERE
        workspace_id = ANY(@ids :: uuid [ ])
    GROUP BY
        workspace_id
) m
JOIN
	 workspace_build_with_user AS wb
ON m.workspace_id = wb.workspace_id AND m.max_build_number = wb.build_number;

-- name: GetLatestWorkspaceBuilds :many
SELECT wb.*
FROM (
    SELECT
        workspace_id, MAX(build_number) as max_build_number
    FROM
		workspace_build_with_user AS workspace_builds
    GROUP BY
        workspace_id
) m
JOIN
	 workspace_build_with_user AS wb
ON m.workspace_id = wb.workspace_id AND m.max_build_number = wb.build_number;

-- name: InsertWorkspaceBuild :exec
INSERT INTO
	workspace_builds (
		id,
		created_at,
		updated_at,
		workspace_id,
		template_version_id,
		"build_number",
		transition,
		initiator_id,
		job_id,
		provisioner_state,
		deadline,
		max_deadline,
		reason
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13);

-- name: UpdateWorkspaceBuildCostByID :exec
UPDATE
	workspace_builds
SET
	daily_cost = $2
WHERE
	id = $1;

-- name: UpdateWorkspaceBuildDeadlineByID :exec
UPDATE
	workspace_builds
SET
	deadline = @deadline::timestamptz,
	max_deadline = @max_deadline::timestamptz,
	updated_at = @updated_at::timestamptz
WHERE id = @id::uuid;

-- name: UpdateWorkspaceBuildProvisionerStateByID :exec
UPDATE
	workspace_builds
SET
	provisioner_state = @provisioner_state::bytea,
	updated_at = @updated_at::timestamptz
WHERE id = @id::uuid;

-- name: GetActiveWorkspaceBuildsByTemplateID :many
SELECT wb.*
FROM (
    SELECT
        workspace_id, MAX(build_number) as max_build_number
    FROM
		workspace_build_with_user AS workspace_builds
    WHERE
        workspace_id IN (
			SELECT
				id
			FROM
				workspaces
			WHERE
				template_id = $1
		)
    GROUP BY
        workspace_id
) m
JOIN
	workspace_build_with_user AS wb
	ON m.workspace_id = wb.workspace_id AND m.max_build_number = wb.build_number
JOIN
	provisioner_jobs AS pj
	ON wb.job_id = pj.id
WHERE
	wb.transition = 'start'::workspace_transition
AND
	pj.completed_at IS NOT NULL;
