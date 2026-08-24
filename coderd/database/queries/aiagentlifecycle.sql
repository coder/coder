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
	ai_agent_lifecycle_journal_create (entry_id, line, creation_site_type, creation_site_id)
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
-- creation_time is the effective date of the entry this row is posted from, so
-- the caller passes the same value it gave that entry rather than a second
-- reading of the clock.
INSERT INTO
	ai_agent_ledger (
		id,
		owner_type,
		owner_id,
		creation_site_type,
		creation_site_id,
		state,
		posting_reference,
		creation_time
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;

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

-- name: GetLiveAIAgentByCreationSite :one
-- The live AI agent of a creation site, if it has one.
--
-- Live means active. Retired is the only other state reachable today, and
-- dormant, which is in the set and unreachable, is not live either, so naming
-- the state wanted rather than the states excluded stays right when it arrives.
SELECT
	*
FROM
	ai_agent_ledger
WHERE
	creation_site_type = $1
	AND creation_site_id = $2
	AND state = 'active';

-- name: GetLatestAIAgentByCreationSite :one
-- The most recently created AI agent of a site whatever its state, so that a
-- caller can tell a site that never had one, which returns no rows, from a site
-- whose agent has been retired.
--
-- A site can have had several over time, retirement freeing it for another, so
-- this orders rather than assuming one.
SELECT
	*
FROM
	ai_agent_ledger
WHERE
	creation_site_type = $1
	AND creation_site_id = $2
ORDER BY
	creation_time DESC
LIMIT
	1;

-- name: GetAIAgentsByOwner :many
-- The AI agents a principal owns, newest first.
--
-- Joined to users only for the mirrored username, which is the one thing the
-- ledger does not hold. The join goes when the mirror does, the name being
-- derived from the identifier either way.
SELECT
	sqlc.embed(ai_agent_ledger),
	users.username
FROM
	ai_agent_ledger
	INNER JOIN users ON users.id = ai_agent_ledger.id
WHERE
	ai_agent_ledger.owner_id = $1
ORDER BY
	ai_agent_ledger.creation_time DESC;
