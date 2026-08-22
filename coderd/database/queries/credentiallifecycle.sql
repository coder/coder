-- name: NextCredentialLifecycleJournalEntryID :one
-- One call per entry. The journal is in the normalized form, so this is the
-- key of the entry table and of any line rows that later join to it.
SELECT
	nextval('credential_lifecycle_journal_entry_seq')::bigint;

-- name: InsertCredentialLifecycleJournalEntry :one
-- recording_date is absent from this statement on purpose: the column default
-- supplies it, so no caller can supply, override, or backdate it.
INSERT INTO
	credential_lifecycle_journal (
		entry_id,
		effective_date,
		actor_type,
		actor,
		event,
		subject
	)
VALUES
	($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: InsertCredentialLifecycleLedgerRow :one
INSERT INTO
	credential_lifecycle_ledger (
		id,
		holder_type,
		holder_id,
		credential_type,
		state,
		expires_at,
		posting_reference
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7) RETURNING *;

-- name: InsertCredentialPassword :one
-- The password type's own state, keyed on the ledger row it belongs to. Written
-- in the same transaction as that row: a ledger row of type password with no
-- row here is a credential nothing can verify.
INSERT INTO
	credential_password (id, hashed_authenticator)
VALUES
	($1, $2) RETURNING *;

-- name: GetCredentialLifecycleLedgerRowByID :one
SELECT
	*
FROM
	credential_lifecycle_ledger
WHERE
	id = $1;

-- name: GetCredentialPasswordByID :one
SELECT
	*
FROM
	credential_password
WHERE
	id = $1;

-- name: GetValidCredentialsByHolder :many
-- Every credential currently valid for one holder. More than one may be valid
-- at a time, so that a rotation can overlap rather than leaving an interval
-- with none.
--
-- State only. Expiry is not considered here: nothing writes expires_at yet, and
-- evaluating it belongs to the work package that does.
--
-- Type specific state is not joined in. A caller that needs it knows the type
-- from the row and fetches it, which is what the type discriminator is for.
SELECT
	*
FROM
	credential_lifecycle_ledger
WHERE
	holder_type = $1
	AND holder_id = $2
	AND state = 'valid';

-- name: RevokeCredential :one
-- Conditional on the posting reference the caller last saw, so that two posters
-- cannot both believe they succeeded.
UPDATE
	credential_lifecycle_ledger
SET
	state = 'invalid',
	posting_reference = $2
WHERE
	id = $1
	AND posting_reference = $3 RETURNING *;

-- name: InsertCredentialAPIKey :one
-- The api_key type's own state, keyed on the ledger row it belongs to and
-- written in the same transaction as it.
INSERT INTO
	credential_api_key (id, hashed_secret, token_name, scopes, allow_list)
VALUES
	($1, $2, $3, $4, $5) RETURNING *;

-- name: GetCredentialAPIKeyByID :one
SELECT
	*
FROM
	credential_api_key
WHERE
	id = $1;

-- name: InsertCredentialLifecycleJournalAPIKeyLine :one
-- What an issuance of an api_key credential carried. The entry says an issuance
-- occurred; this says with what.
INSERT INTO
	credential_lifecycle_journal_api_key (entry_id, line, token_name, scopes, allow_list)
VALUES
	($1, $2, $3, $4, $5) RETURNING *;

-- name: GetCredentialLifecycleJournalAPIKeyLines :many
-- The api_key lines of one entry, in line order.
SELECT
	*
FROM
	credential_lifecycle_journal_api_key
WHERE
	entry_id = $1
ORDER BY
	line;
