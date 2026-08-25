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
-- A line after the first. Live since 2026-08-25: retiring an AI agent ends every
-- authorization naming it, which is one event and so one entry with a line
-- apiece. Revoke and grant, the other case that wants a multiline entry, is
-- still out of scope. Every entry level column is written as a literal
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

-- name: InsertAuthorizationLedgerRow :one
INSERT INTO
	authorization_ledger (
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

-- name: TerminateAuthorization :one
-- Post an authorization to `terminated`. Three transitions reach that state,
-- `revoke`, `lapse` and the reserved `disqualify`, so this is named for the
-- posting rather than for any of them. Which one occurred is the entry's
-- business.
--
-- Conditional on the posting reference the caller last saw, so that two posters
-- cannot both believe they succeeded.
UPDATE
	authorization_ledger
SET
	state = 'terminated',
	posting_reference = $2
WHERE
	id = $1
	AND posting_reference = $3 RETURNING *;

-- name: GetAuthorizationLedgerRowByID :one
SELECT
	*
FROM
	authorization_ledger
WHERE
	id = $1;

-- name: GetAuthorizationLifecycleJournalEntriesBySubject :many
-- Entries about one authorization, ordered as they were made. Unlike the AI
-- agent's, this machine has no cycle, so one subject's entries are bounded by
-- the sequences it allows and the limit only caps what a caller will take.
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

-- name: GetAuthorizationLedgerRowsByAgent :many
-- Every authorization held by one agent, whatever its state.
--
-- This exists for `lapse`. When an AI agent reaches `retired`, every
-- authorization naming it as agent must reach `terminated`. Where the
-- retirement is ours to record the two go in one transaction, arising
-- together. Where it is not, an end of life nothing reported has to be found
-- by a sweep instead. See "What the existence of the parties requires" in
-- poc_audit/entity_model.md. The in-transaction route is
-- built; the sweep is not.
--
-- Unlike the credential equivalent it does not filter to the live rows, since
-- both callers have to tell an authorization that already ended from one they
-- must end.
SELECT
	*
FROM
	authorization_ledger
WHERE
	agent_type = $1
	AND agent_id = $2
ORDER BY
	posting_reference;
