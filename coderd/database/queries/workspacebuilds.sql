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
		reason,
		template_version_preset_id
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14);

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

-- name: GetWorkspaceBuildStatsByTemplates :many
SELECT
    w.template_id,
	t.name AS template_name,
	t.display_name AS template_display_name,
	t.organization_id AS template_organization_id,
    COUNT(*) AS total_builds,
    COUNT(CASE WHEN pj.job_status = 'failed' THEN 1 END) AS failed_builds
FROM
    workspace_build_with_user AS wb
JOIN
    workspaces AS w ON
    wb.workspace_id = w.id
JOIN
    provisioner_jobs AS pj ON
    wb.job_id = pj.id
JOIN
    templates AS t ON
	w.template_id = t.id
WHERE
    wb.created_at >= @since
    AND pj.completed_at IS NOT NULL
GROUP BY
    w.template_id, template_name, template_display_name, template_organization_id
ORDER BY
    template_name ASC;

-- name: GetFailedWorkspaceBuildsByTemplateID :many
SELECT
	tv.name AS template_version_name,
	u.username AS workspace_owner_username,
	w.name AS workspace_name,
	wb.build_number AS workspace_build_number
FROM
	workspace_build_with_user AS wb
JOIN
	workspaces AS w
ON
	wb.workspace_id = w.id
JOIN
    users AS u
ON
    w.owner_id = u.id
JOIN
	provisioner_jobs AS pj
ON
	wb.job_id = pj.id
JOIN
	templates AS t
ON
	w.template_id = t.id
JOIN
	template_versions AS tv
ON
	wb.template_version_id = tv.id
WHERE
	w.template_id = $1
	AND wb.created_at >= @since
	AND pj.completed_at IS NOT NULL
	AND pj.job_status = 'failed'
ORDER BY
	tv.name ASC, wb.build_number DESC;
