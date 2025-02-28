-- Acquires the lock for a single job that isn't started, completed,
-- canceled, and that matches an array of provisioner types.
--
-- SKIP LOCKED is used to jump over locked rows. This prevents
-- multiple provisioners from acquiring the same jobs. See:
-- https://www.postgresql.org/docs/9.5/sql-select.html#SQL-FOR-UPDATE-SHARE
-- name: AcquireProvisionerJob :one
UPDATE
	provisioner_jobs
SET
	started_at = @started_at,
	updated_at = @started_at,
	worker_id = @worker_id
WHERE
	id = (
		SELECT
			id
		FROM
			provisioner_jobs AS potential_job
		WHERE
			potential_job.started_at IS NULL
			AND potential_job.organization_id = @organization_id
			-- Ensure the caller has the correct provisioner.
			AND potential_job.provisioner = ANY(@types :: provisioner_type [ ])
			-- elsewhere, we use the tagset type, but here we use jsonb for backward compatibility
			-- they are aliases and the code that calls this query already relies on a different type
			AND provisioner_tagset_contains(@provisioner_tags :: jsonb, potential_job.tags :: jsonb)
		ORDER BY
			potential_job.created_at
		FOR UPDATE
		SKIP LOCKED
		LIMIT
			1
	) RETURNING *;

-- name: GetProvisionerJobByID :one
SELECT
	*
FROM
	provisioner_jobs
WHERE
	id = $1;

-- name: GetProvisionerJobsByIDs :many
SELECT
	*
FROM
	provisioner_jobs
WHERE
	id = ANY(@ids :: uuid [ ]);

-- name: GetProvisionerJobsByIDsWithQueuePosition :many
WITH filtered_provisioner_jobs AS (
	-- Step 1: Filter provisioner_jobs
	SELECT
		id, created_at
	FROM
		provisioner_jobs
	WHERE
		id = ANY(@ids :: uuid [ ]) -- Apply filter early to reduce dataset size before expensive JOIN
),
pending_jobs AS (
	-- Step 2: Extract only pending jobs
	SELECT
		id, created_at, tags
	FROM
		provisioner_jobs
	WHERE
		job_status = 'pending'
),
ranked_jobs AS (
	-- Step 3: Rank only pending jobs based on provisioner availability
	SELECT
		pj.id,
		pj.created_at,
		ROW_NUMBER() OVER (PARTITION BY pd.id ORDER BY pj.created_at ASC) AS queue_position,
		COUNT(*) OVER (PARTITION BY pd.id) AS queue_size
	FROM
		pending_jobs pj
			INNER JOIN provisioner_daemons pd
					ON provisioner_tagset_contains(pd.tags, pj.tags) -- Join only on the small pending set
),
final_jobs AS (
	-- Step 4: Compute best queue position and max queue size per job
	SELECT
		fpj.id,
		fpj.created_at,
		COALESCE(MIN(rj.queue_position), 0) :: BIGINT AS queue_position, -- Best queue position across provisioners
		COALESCE(MAX(rj.queue_size), 0) :: BIGINT AS queue_size -- Max queue size across provisioners
	FROM
		filtered_provisioner_jobs fpj -- Use the pre-filtered dataset instead of full provisioner_jobs
			LEFT JOIN ranked_jobs rj
					ON fpj.id = rj.id -- Join with the ranking jobs CTE to assign a rank to each specified provisioner job.
	GROUP BY
		fpj.id, fpj.created_at
)
SELECT
	-- Step 5: Final SELECT with INNER JOIN provisioner_jobs
	fj.id,
	fj.created_at,
	sqlc.embed(pj),
	fj.queue_position,
	fj.queue_size
FROM
	final_jobs fj
		INNER JOIN provisioner_jobs pj
				ON fj.id = pj.id -- Ensure we retrieve full details from `provisioner_jobs`.
                                 -- JOIN with pj is required for sqlc.embed(pj) to compile successfully.
ORDER BY
	fj.created_at;

-- name: GetProvisionerJobsByOrganizationAndStatusWithQueuePositionAndProvisioner :many
WITH pending_jobs AS (
    SELECT
        id, created_at
    FROM
        provisioner_jobs
    WHERE
        started_at IS NULL
    AND
        canceled_at IS NULL
    AND
        completed_at IS NULL
    AND
        error IS NULL
),
queue_position AS (
    SELECT
        id,
        ROW_NUMBER() OVER (ORDER BY created_at ASC) AS queue_position
    FROM
        pending_jobs
),
queue_size AS (
	SELECT COUNT(*) AS count FROM pending_jobs
)
SELECT
	sqlc.embed(pj),
    COALESCE(qp.queue_position, 0) AS queue_position,
    COALESCE(qs.count, 0) AS queue_size,
	-- Use subquery to utilize ORDER BY in array_agg since it cannot be
	-- combined with FILTER.
	(
		SELECT
			-- Order for stable output.
			array_agg(pd.id ORDER BY pd.created_at ASC)::uuid[]
		FROM
			provisioner_daemons pd
		WHERE
			-- See AcquireProvisionerJob.
			pj.started_at IS NULL
			AND pj.organization_id = pd.organization_id
			AND pj.provisioner = ANY(pd.provisioners)
			AND provisioner_tagset_contains(pd.tags, pj.tags)
	) AS available_workers,
	-- Include template and workspace information.
	COALESCE(tv.name, '') AS template_version_name,
	t.id AS template_id,
	COALESCE(t.name, '') AS template_name,
	COALESCE(t.display_name, '') AS template_display_name,
	COALESCE(t.icon, '') AS template_icon,
	w.id AS workspace_id,
	COALESCE(w.name, '') AS workspace_name
FROM
	provisioner_jobs pj
LEFT JOIN
	queue_position qp ON qp.id = pj.id
LEFT JOIN
	queue_size qs ON TRUE
LEFT JOIN
	workspace_builds wb ON wb.id = CASE WHEN pj.input ? 'workspace_build_id' THEN (pj.input->>'workspace_build_id')::uuid END
LEFT JOIN
	workspaces w ON (
		w.id = wb.workspace_id
		AND w.organization_id = pj.organization_id
	)
LEFT JOIN
	-- We should always have a template version, either explicitly or implicitly via workspace build.
	template_versions tv ON (
		tv.id = CASE WHEN pj.input ? 'template_version_id' THEN (pj.input->>'template_version_id')::uuid ELSE wb.template_version_id END
		AND tv.organization_id = pj.organization_id
	)
LEFT JOIN
	templates t ON (
		t.id = tv.template_id
		AND t.organization_id = pj.organization_id
	)
WHERE
	pj.organization_id = @organization_id::uuid
	AND (COALESCE(array_length(@ids::uuid[], 1), 0) = 0 OR pj.id = ANY(@ids::uuid[]))
	AND (COALESCE(array_length(@status::provisioner_job_status[], 1), 0) = 0 OR pj.job_status = ANY(@status::provisioner_job_status[]))
	AND (@tags::tagset = 'null'::tagset OR provisioner_tagset_contains(pj.tags::tagset, @tags::tagset))
GROUP BY
	pj.id,
	qp.queue_position,
	qs.count,
	tv.name,
	t.id,
	t.name,
	t.display_name,
	t.icon,
	w.id,
	w.name
ORDER BY
	pj.created_at DESC
LIMIT
	sqlc.narg('limit')::int;

-- name: GetProvisionerJobsCreatedAfter :many
SELECT * FROM provisioner_jobs WHERE created_at > $1;

-- name: InsertProvisionerJob :one
INSERT INTO
	provisioner_jobs (
		id,
		created_at,
		updated_at,
		organization_id,
		initiator_id,
		provisioner,
		storage_method,
		file_id,
		"type",
		"input",
		tags,
		trace_metadata
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING *;

-- name: UpdateProvisionerJobByID :exec
UPDATE
	provisioner_jobs
SET
	updated_at = $2
WHERE
	id = $1;

-- name: UpdateProvisionerJobWithCancelByID :exec
UPDATE
	provisioner_jobs
SET
	canceled_at = $2,
	completed_at = $3
WHERE
	id = $1;

-- name: UpdateProvisionerJobWithCompleteByID :exec
UPDATE
	provisioner_jobs
SET
	updated_at = $2,
	completed_at = $3,
	error = $4,
	error_code = $5
WHERE
	id = $1;

-- name: GetHungProvisionerJobs :many
SELECT
	*
FROM
	provisioner_jobs
WHERE
	updated_at < $1
	AND started_at IS NOT NULL
	AND completed_at IS NULL;

-- name: InsertProvisionerJobTimings :many
INSERT INTO provisioner_job_timings (job_id, started_at, ended_at, stage, source, action, resource)
SELECT
    @job_id::uuid AS provisioner_job_id,
    unnest(@started_at::timestamptz[]),
    unnest(@ended_at::timestamptz[]),
    unnest(@stage::provisioner_job_timing_stage[]),
    unnest(@source::text[]),
    unnest(@action::text[]),
    unnest(@resource::text[])
RETURNING *;

-- name: GetProvisionerJobTimingsByJobID :many
SELECT * FROM provisioner_job_timings
WHERE job_id = $1
ORDER BY started_at ASC;
