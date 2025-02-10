-- name: GetProvisionerDaemons :many
SELECT
	*
FROM
	provisioner_daemons;

-- name: GetProvisionerDaemonsByOrganization :many
SELECT
	*
FROM
	provisioner_daemons
WHERE
	-- This is the original search criteria:
	organization_id = @organization_id :: uuid
	AND
	-- adding support for searching by tags:
	(@want_tags :: tagset = 'null' :: tagset OR provisioner_tagset_contains(provisioner_daemons.tags::tagset, @want_tags::tagset));

-- name: GetEligibleProvisionerDaemonsByProvisionerJobIDs :many
SELECT DISTINCT
    provisioner_jobs.id as job_id, sqlc.embed(provisioner_daemons)
FROM
    provisioner_jobs
JOIN
    provisioner_daemons ON provisioner_daemons.organization_id = provisioner_jobs.organization_id
    AND provisioner_tagset_contains(provisioner_daemons.tags::tagset, provisioner_jobs.tags::tagset)
    AND provisioner_jobs.provisioner = ANY(provisioner_daemons.provisioners)
WHERE
    provisioner_jobs.id = ANY(@provisioner_job_ids :: uuid[]);

-- name: GetProvisionerDaemonsWithStatusByOrganization :many
SELECT
	sqlc.embed(pd),
	CASE
		WHEN pd.last_seen_at IS NULL OR pd.last_seen_at < (NOW() - (@stale_interval_ms::bigint || ' ms')::interval)
		THEN 'offline'
		ELSE CASE
			WHEN current_job.id IS NOT NULL THEN 'busy'
			ELSE 'idle'
		END
	END::provisioner_daemon_status AS status,
	pk.name AS key_name,
	-- NOTE(mafredri): sqlc.embed doesn't support nullable tables nor renaming them.
	current_job.id AS current_job_id,
	current_job.job_status AS current_job_status,
	previous_job.id AS previous_job_id,
	previous_job.job_status AS previous_job_status,
	tmpl.name AS template_name,
	tmpl.display_name AS template_display_name,
	tmpl.icon AS template_icon
FROM
	provisioner_daemons pd
JOIN
	provisioner_keys pk ON pk.id = pd.key_id
LEFT JOIN
	provisioner_jobs current_job ON (
		current_job.worker_id = pd.id
		AND current_job.completed_at IS NULL
	)
LEFT JOIN
	provisioner_jobs previous_job ON (
		previous_job.id = (
			SELECT
				id
			FROM
				provisioner_jobs
			WHERE
				worker_id = pd.id
				AND completed_at IS NOT NULL
			ORDER BY
				completed_at DESC
			LIMIT 1
		)
	)
JOIN
	template_versions version ON version.id::text = pd.version
LEFT JOIN
	templates tmpl ON tmpl.id = version.template_id
WHERE
	pd.organization_id = @organization_id::uuid
	AND (COALESCE(array_length(@ids::uuid[], 1), 0) = 0 OR pd.id = ANY(@ids::uuid[]))
	AND (@tags::tagset = 'null'::tagset OR provisioner_tagset_contains(pd.tags::tagset, @tags::tagset))
ORDER BY
	pd.created_at ASC;

-- name: DeleteOldProvisionerDaemons :exec
-- Delete provisioner daemons that have been created at least a week ago
-- and have not connected to coderd since a week.
-- A provisioner daemon with "zeroed" last_seen_at column indicates possible
-- connectivity issues (no provisioner daemon activity since registration).
DELETE FROM provisioner_daemons WHERE (
	(created_at < (NOW() - INTERVAL '7 days') AND last_seen_at IS NULL) OR
	(last_seen_at IS NOT NULL AND last_seen_at < (NOW() - INTERVAL '7 days'))
);

-- name: UpsertProvisionerDaemon :one
INSERT INTO
	provisioner_daemons (
		id,
		created_at,
		"name",
		provisioners,
		tags,
		last_seen_at,
		"version",
		organization_id,
		api_version,
		key_id
	)
VALUES (
	gen_random_uuid(),
	@created_at,
	@name,
	@provisioners,
	@tags,
	@last_seen_at,
	@version,
	@organization_id,
	@api_version,
	@key_id
) ON CONFLICT("organization_id", "name", LOWER(COALESCE(tags ->> 'owner'::text, ''::text))) DO UPDATE SET
	provisioners = @provisioners,
	tags = @tags,
	last_seen_at = @last_seen_at,
	"version" = @version,
	api_version = @api_version,
	organization_id = @organization_id,
	key_id = @key_id
RETURNING *;

-- name: UpdateProvisionerDaemonLastSeenAt :exec
UPDATE provisioner_daemons
SET
	last_seen_at = @last_seen_at
WHERE
	id = @id
AND
	last_seen_at <= @last_seen_at;
