-- name: NextAIAgentLifecycleJournalEntryID :one
-- One call per entry, whose value every line of that entry then carries.
SELECT
	nextval('ai_agent_lifecycle_journal_entry_seq')::bigint;

-- name: InsertAIAgentLifecycleJournalEntry :one
-- recording_date is absent from this statement on purpose: the column default
-- supplies it, so no caller can supply, override, or backdate it.
INSERT INTO
	ai_agent_lifecycle_journal (
		entry_id,
		effective_date,
		actor_type,
		actor,
		event,
		subject
	)
VALUES
	($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: InsertAIAgentLifecycleJournalCreateLine :one
-- What a creation carried. Line zero, this being the only line, and the only
-- line table this journal has until `transfer` gives it a second shape.
INSERT INTO
	ai_agent_lifecycle_journal_create (entry_id, line, origin_type, origin_id)
VALUES
	($1, $2, $3, $4) RETURNING *;

-- name: GetAIAgentLifecycleJournalCreateLines :many
-- The lines of one creation entry, ordered as they were written.
SELECT
	*
FROM
	ai_agent_lifecycle_journal_create
WHERE
	entry_id = $1
ORDER BY
	line;

-- name: InsertAIAgentLedgerRow :one
INSERT INTO
	ai_agent_ledger (
		id,
		owner_type,
		owner_id,
		origin_type,
		origin_id,
		state,
		posting_reference
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7) RETURNING *;

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
	entry_id
LIMIT
	$2;
