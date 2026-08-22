-- name: NextAIAgentLifecycleJournalEntryID :one
-- One call per entry, whose value every line of that entry then carries.
SELECT
	nextval('ai_agent_lifecycle_journal_entry_seq')::bigint;

-- name: InsertAIAgentLifecycleJournalFirstLine :one
-- Line 0 carries the entry level values. recording_date is absent from this
-- statement on purpose: the column default supplies it, so no caller can
-- supply, override, or backdate it.
INSERT INTO
	ai_agent_lifecycle_journal (
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

-- name: InsertAIAgentLifecycleJournalSubsequentLine :one
-- NOT LIVE CODE. Nothing calls this. It is here to show what a line after the
-- first looks like, since the proof of concept writes no multiline entry for an
-- AI agent. It deserves a unit test of its own and does not have one, so treat
-- it as documentation rather than as a tested path. In production this would
-- rot; this is not production.
INSERT INTO
	ai_agent_lifecycle_journal (
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

-- name: InsertAIAgentLedgerRow :one
INSERT INTO
	ai_agent_ledger (
		id,
		owner_type,
		owner_id,
		state,
		posting_reference
	)
VALUES
	($1, $2, $3, $4, $5) RETURNING *;

-- name: GetAIAgentLedgerRowByID :one
SELECT
	*
FROM
	ai_agent_ledger
WHERE
	id = $1;

-- name: RetireAIAgent :one
-- Posting a retirement. Conditioned on the posting reference the caller expects
-- to find, so that two concurrent posters cannot both believe they succeeded.
UPDATE
	ai_agent_ledger
SET
	state = 'retired',
	posting_reference = $2
WHERE
	id = $1
	AND posting_reference = $3 RETURNING *;

-- name: GetAIAgentLifecycleEntriesBySubject :many
-- Entries about one AI agent, ordered as they were made. This machine has a
-- cycle, `transfer` being a self-transition, so one subject can accumulate
-- entries without limit and the bound is one the caller chooses rather than one
-- the machine guarantees. Callers pass one more than they will accept, so
-- receiving it tells them the set was larger.
SELECT
	*
FROM
	ai_agent_lifecycle_journal
WHERE
	subject = $1
ORDER BY
	entry_id,
	line
LIMIT
	$2;
