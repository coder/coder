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
