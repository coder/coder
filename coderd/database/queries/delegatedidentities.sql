-- name: InsertDelegatedIdentity :one
INSERT INTO delegated_identities (
	id,
	organization_id,
	parent_id,
	root_user_id,
	created_by,
	name,
	scopes,
	created_at,
	updated_at
)
VALUES (
	@id,
	@organization_id,
	@parent_id,
	@root_user_id,
	@created_by,
	@name,
	@scopes,
	@created_at,
	@created_at
)
RETURNING *;

-- GetDelegatedIdentityByID also returns identities whose account was
-- deleted, so that audit entries can still be traced to a root user.
-- name: GetDelegatedIdentityByID :one
SELECT
	*
FROM
	delegated_identities
WHERE
	id = @id;

-- GetDelegatedIdentityChain returns a delegated identity and every delegated
-- identity above it, nearest first. The parent of the last row is the root
-- user. It returns no rows for an account that is not a delegated identity.
-- name: GetDelegatedIdentityChain :many
WITH RECURSIVE chain AS (
	SELECT
		delegated_identities.id,
		delegated_identities.parent_id,
		1 AS depth
	FROM
		delegated_identities
	WHERE
		delegated_identities.id = @id
	UNION ALL
	SELECT
		parent.id,
		parent.parent_id,
		chain.depth + 1
	FROM
		delegated_identities parent
		JOIN chain ON chain.parent_id = parent.id
	-- A chain cannot loop, since a parent exists before its child. The
	-- limit only bounds the damage of a corrupt row.
	WHERE
		chain.depth < 32
)
SELECT
	delegated_identities.*
FROM
	delegated_identities
	JOIN chain ON chain.id = delegated_identities.id
ORDER BY
	chain.depth ASC;

-- GetDelegatedIdentitiesByAncestorID returns the live delegated identities
-- below a user or delegated identity, at any depth, oldest first.
-- name: GetDelegatedIdentitiesByAncestorID :many
WITH RECURSIVE descendants AS (
	SELECT
		delegated_identities.id,
		1 AS depth
	FROM
		delegated_identities
	WHERE
		delegated_identities.parent_id = @ancestor_id
	UNION ALL
	SELECT
		child.id,
		descendants.depth + 1
	FROM
		delegated_identities child
		JOIN descendants ON child.parent_id = descendants.id
	WHERE
		descendants.depth < 32
)
SELECT
	delegated_identities.*
FROM
	delegated_identities
	JOIN descendants ON descendants.id = delegated_identities.id
	JOIN users ON users.id = delegated_identities.id
WHERE
	NOT users.deleted
ORDER BY
	delegated_identities.created_at ASC,
	delegated_identities.id ASC;

-- name: UpdateDelegatedIdentityScopes :one
UPDATE
	delegated_identities
SET
	scopes = @scopes,
	updated_at = @updated_at
WHERE
	id = @id
RETURNING *;

-- GetDelegatedIdentityUsage counts what keeps a delegated identity from
-- being deleted: the workspaces and chats it owns and the live identities
-- below it.
-- name: GetDelegatedIdentityUsage :one
SELECT
	(
		SELECT
			COUNT(*)
		FROM
			workspaces
		WHERE
			workspaces.owner_id = @id
			AND NOT workspaces.deleted
	) AS workspaces,
	(
		SELECT
			COUNT(*)
		FROM
			chats
		WHERE
			chats.owner_id = @id
	) AS chats,
	(
		SELECT
			COUNT(*)
		FROM
			delegated_identities child
			JOIN users ON users.id = child.id
		WHERE
			child.parent_id = @id
			AND NOT users.deleted
	) AS children;

-- DeleteOrphanedDelegatedIdentities deletes the accounts of delegated
-- identities that own no workspace and no chat, have no live identity below
-- them, and were created before the given time. The rows in delegated_identities
-- stay, so that audit entries can still be traced to a root user.
-- name: DeleteOrphanedDelegatedIdentities :many
UPDATE
	users
SET
	deleted = true
WHERE
	NOT users.deleted
	AND users.id IN (
		SELECT
			delegated_identities.id
		FROM
			delegated_identities
		WHERE
			delegated_identities.created_at < @created_before
			AND NOT EXISTS (
				SELECT
					1
				FROM
					workspaces
				WHERE
					workspaces.owner_id = delegated_identities.id
					AND NOT workspaces.deleted
			)
			AND NOT EXISTS (
				SELECT
					1
				FROM
					chats
				WHERE
					chats.owner_id = delegated_identities.id
			)
			AND NOT EXISTS (
				SELECT
					1
				FROM
					delegated_identities child
					JOIN users child_users ON child_users.id = child.id
				WHERE
					child.parent_id = delegated_identities.id
					AND NOT child_users.deleted
			)
	)
RETURNING users.id;
