-- name: NextCredentialLifecycleJournalEntryID :one
-- One call per entry, whose value every line of that entry then carries.
SELECT
	nextval('credential_lifecycle_journal_entry_seq')::bigint;

-- name: InsertCredentialLifecycleJournalFirstLine :one
-- Line 0 carries the entry level values. recording_date is absent from this
-- statement on purpose: the column default supplies it, so no caller can
-- supply, override, or backdate it.
INSERT INTO
	credential_lifecycle_journal (
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

-- name: InsertCredentialLifecycleJournalSubsequentLine :one
-- NOT LIVE CODE. Nothing calls this. It is here to show what a line after the
-- first looks like, since the proof of concept writes no multiline entry:
-- rotation, the case that needs one, is out of scope. It deserves a unit test
-- of its own and does not have one, so treat it as documentation rather than
-- as a tested path. In production this would rot; this is not production.
INSERT INTO
	credential_lifecycle_journal (
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

-- name: InsertCredentialLifecycleLedgerRow :one
INSERT INTO
	credential_lifecycle_ledger (
		id,
		holder_type,
		holder_id,
		credential_type,
		credential_value,
		state,
		expires_at,
		posting_reference
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;

-- name: GetCredentialLifecycleLedgerRowByID :one
SELECT
	*
FROM
	credential_lifecycle_ledger
WHERE
	id = $1;

-- name: GetValidCredentialsByHolder :many
-- Every credential currently valid for one holder. More than one may be valid
-- at a time, so that a rotation can overlap rather than leaving an interval
-- with none.
--
-- State only. Expiry is not considered here: nothing writes expires_at yet, and
-- evaluating it belongs to the work package that does.
SELECT
	*
FROM
	credential_lifecycle_ledger
WHERE
	holder_type = $1
	AND holder_id = $2
	AND state = 'valid';

-- name: RevokeCredential :one
-- Posting a revocation. Conditioned on the posting reference the caller expects
-- to find, so that two concurrent posters cannot both believe they succeeded.
UPDATE
	credential_lifecycle_ledger
SET
	state = 'invalid',
	posting_reference = $2
WHERE
	id = $1
	AND posting_reference = $3 RETURNING *;
