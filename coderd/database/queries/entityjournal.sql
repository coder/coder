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
-- Entries about one entity, which is what makes the limit meaningful. A
-- lifecycle is a state machine without cycles, so one entity's entries are
-- bounded by the sequences that machine allows. Callers pass one entry more
-- than they will accept, so that receiving it tells them the set was larger
-- than that.
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
