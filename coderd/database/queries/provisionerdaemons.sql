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
	(@organization_id :: uuid IS NULL OR organization_id = @organization_id :: uuid)
	AND
	-- adding support for searching by tags:
	(@tags :: jsonb IS NULL OR tags_compatible(@tags :: jsonb, provisioner_daemons.tags :: jsonb))
	AND
	-- Because we're adding @tags as a second search parameter, we need to do this check to
	-- ensure that the first parameter's behavior remains unchanged when no second parameter is provided:
	(@organization_id :: uuid IS NOT NULL OR @tags IS NOT NULL);

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
