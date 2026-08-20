-- name: NextAuthorizationLifecycleJournalEntryID :one
-- One call per entry, whose value every line of that entry then carries. A
-- column default cannot serve, allocating per row where the lines of an entry
-- must agree.
SELECT
	nextval('authorization_lifecycle_journal_entry_seq')::bigint;

-- name: InsertAuthorizationLifecycleJournalFirstLine :one
-- Line 0 carries the entry level values. recording_date is absent from this
-- statement on purpose: the column default supplies it, so no caller can
-- supply, override, or backdate it.
INSERT INTO
	authorization_lifecycle_journal (
		entry_id,
		line,
		effective_date,
		actor_type,
		actor,
		event,
		subject
	)
VALUES
	($1, 0, $2, $3, $4, $5, $6) RETURNING *;

-- name: InsertAuthorizationLifecycleJournalSubsequentLine :one
-- NOT LIVE CODE. Nothing calls this. It is here to show what a line after the
-- first looks like, since the proof of concept writes no multiline entry:
-- revoke and grant, the case that needs one, is out of scope. It deserves a
-- unit test of its own and does not have one, so treat it as documentation
-- rather than as a tested path. In production this would rot; this is not
-- production.
--
-- A line after the first. Every entry level column is written as a literal
-- null rather than as a parameter, for the same reason line 0 omits the
-- recording date: what a caller cannot name, a caller cannot get wrong.
INSERT INTO
	authorization_lifecycle_journal (
		entry_id,
		line,
		recording_date,
		effective_date,
		actor_type,
		actor,
		event,
		subject
	)
VALUES
	($1, $2, NULL, NULL, NULL, NULL, $3, $4) RETURNING *;

-- name: InsertAuthorizationLifecycleLedgerRow :one
INSERT INTO
	authorization_lifecycle_ledger (
		id,
		principal_type,
		principal_id,
		agent_type,
		agent_id,
		scope,
		state,
		posting_reference
	)
VALUES
	($1, $2, $3, $4, $5, '', $6, $7) RETURNING *;

-- name: GetAuthorizationLifecycleLedgerRowByID :one
SELECT
	*
FROM
	authorization_lifecycle_ledger
WHERE
	id = $1;

-- name: GetAuthorizationLifecycleJournalEntriesBySubject :many
-- Entries about one authorization, ordered as they were made. Bounded for the
-- same reason as the AI agent journal: a lifecycle is a state machine without
-- cycles, so one subject's entries are bounded by the sequences it allows.
-- Callers pass one more than they will accept, so receiving it tells them the
-- set was larger.
SELECT
	*
FROM
	authorization_lifecycle_journal
WHERE
	subject = $1
ORDER BY
	entry_id,
	line
LIMIT
	$2;
