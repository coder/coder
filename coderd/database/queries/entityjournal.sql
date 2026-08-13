-- name: InsertEntityJournalEntry :one
INSERT INTO
	entity_journal (
		recorded_at,
		event,
		subject_type,
		subject,
		actor_type,
		actor
	)
VALUES
	($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: GetLifecycleEntriesBySubject :many
-- The limit is a backstop rather than pagination. Callers pass one entry more
-- than they will accept, so that receiving it tells them the set was larger
-- than an entity's lifecycle can produce.
SELECT
	*
FROM
	entity_journal
WHERE
	subject_type = $1
	AND subject = $2
ORDER BY
	id
LIMIT
	$3;

-- name: GetLifecycleEntriesByActor :many
SELECT
	*
FROM
	entity_journal
WHERE
	actor_type = $1
	AND actor = $2
ORDER BY
	id
LIMIT
	$3;
