-- name: GetProvisionerDaemons :many
SELECT
	*
FROM
	provisioner_daemons;

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
		"version"
	)
VALUES (
	gen_random_uuid(),
	@created_at,
	@name,
	@provisioners,
	@tags,
	@last_seen_at,
	@version
) ON CONFLICT("name", lower((tags ->> 'owner'::text))) DO UPDATE SET
	provisioners = @provisioners,
	tags = @tags,
	last_seen_at = @last_seen_at,
	"version" = @version
WHERE
	-- Only ones with the same tags are allowed clobber
	provisioner_daemons.tags <@ @tags :: jsonb
RETURNING *;
