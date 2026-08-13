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

-- name: GetEntityJournalEntriesBySubject :many
SELECT
	*
FROM
	entity_journal
WHERE
	subject_type = $1
	AND subject = $2
ORDER BY
	id;

-- name: GetEntityJournalEntriesByActor :many
SELECT
	*
FROM
	entity_journal
WHERE
	actor_type = $1
	AND actor = $2
ORDER BY
	id;
