-- name: GetGroupByID :one
SELECT
	*
FROM
	groups
WHERE
	id = $1
LIMIT
	1;

-- name: ValidateGroupIDs :one
WITH input AS (
	SELECT
		unnest(@group_ids::uuid[]) AS id
)
SELECT
	array_agg(input.id)::uuid[] as invalid_group_ids,
	COUNT(*) = 0 as ok
FROM
	-- Preserve rows where there is not a matching left (groups) row for each
	-- right (input) row...
	groups
	RIGHT JOIN input ON groups.id = input.id
WHERE
	-- ...so that we can retain exactly those rows where an input ID does not
	-- match an existing group.
	groups.id IS NULL;

-- name: GetGroupByOrgAndName :one
SELECT
	*
FROM
	groups
WHERE
	organization_id = $1
AND
	name = $2
LIMIT
	1;

-- name: GetGroups :many
SELECT
		sqlc.embed(groups),
		organizations.name AS organization_name,
		organizations.display_name AS organization_display_name
FROM
		groups
INNER JOIN
		organizations ON groups.organization_id = organizations.id
WHERE
		true
		AND CASE
				WHEN @organization_id:: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
						groups.organization_id = @organization_id
				ELSE true
		END
		AND CASE
				-- Filter to only include groups a user is a member of
				WHEN @has_member_id::uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
						EXISTS (
								SELECT
										1
								FROM
										-- this view handles the 'everyone' group in orgs.
										group_members_expanded
								WHERE
										group_members_expanded.group_id = groups.id
								AND
										group_members_expanded.user_id = @has_member_id
						)
				ELSE true
		END
		AND CASE WHEN array_length(@group_names :: text[], 1) > 0  THEN
				groups.name = ANY(@group_names)
			ELSE true
		END
		AND CASE WHEN array_length(@group_ids :: uuid[], 1) > 0  THEN
				groups.id = ANY(@group_ids)
			ELSE true
		END
;

-- name: InsertGroup :one
INSERT INTO groups (
	id,
	name,
	display_name,
	organization_id,
	avatar_url,
	quota_allowance
)
VALUES
	($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: InsertMissingGroups :many
-- Inserts any group by name that does not exist. All new groups are given
-- a random uuid, are inserted into the same organization. They have the default
-- values for avatar, display name, and quota allowance (all zero values).
INSERT INTO groups (
	id,
	name,
	organization_id,
						source
)
SELECT
						gen_random_uuid(),
						group_name,
						@organization_id,
						@source
FROM
						UNNEST(@group_names :: text[]) AS group_name
-- If the name conflicts, do nothing.
ON CONFLICT DO NOTHING
RETURNING *;


-- We use the organization_id as the id
-- for simplicity since all users is
-- every member of the org.
-- name: InsertAllUsersGroup :one
INSERT INTO groups (
	id,
	name,
	organization_id
)
VALUES
	(sqlc.arg(organization_id), 'Everyone', sqlc.arg(organization_id)) RETURNING *;

-- name: UpdateGroupByID :one
UPDATE
	groups
SET
	name = @name,
	display_name = @display_name,
	avatar_url = @avatar_url,
	quota_allowance = @quota_allowance
WHERE
	id = @id
RETURNING *;

-- name: DeleteGroupByID :exec
DELETE FROM
	groups
WHERE
	id = $1;
