-- name: NextCredentialLifecycleJournalEntryID :one
-- One call per entry. The journal is in the normalized form, so this is the
-- key of the entry table and of any line rows that later join to it.
SELECT
	nextval('credential_lifecycle_journal_entry_seq')::bigint;

-- name: InsertCredentialLifecycleJournalEntry :one
-- recording_date is absent from this statement on purpose: the column default
-- supplies it, so no caller can supply, override, or backdate it.
--
-- actor is absent for an entailed operation, and entailed_by_entry or
-- entailed_by_annotation says what entailed it. Exactly one of those two is
-- present on an entailed entry and neither is on a commanded one, which the
-- table's checks enforce.
INSERT INTO
	credential_lifecycle_journal (
		entry_id,
		effective_date,
		actor_type,
		actor,
		entailed_by_entry,
		entailed_by_annotation
	)
VALUES
	($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: InsertCredentialLifecycleJournalLine :one
-- One credential this entry acts on, and what happened to it. An entry with
-- several lines is an atomic group: a rotation issues one credential and
-- revokes another as a single event, so that no interval passes without a
-- valid one.
--
-- Line numbers start at zero and are the caller's to assign, being an ordering
-- within the entry rather than a fact about anything outside it.
INSERT INTO
	credential_lifecycle_journal_line (entry_id, line, subject, event)
VALUES
	($1, $2, $3, $4) RETURNING *;

-- name: InsertCredentialLedgerRow :one
INSERT INTO
	credential_ledger (
		id,
		holder_type,
		holder_id,
		credential_type,
		state,
		expires_at,
		lifecycle_posting_reference
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

-- name: GetCredentialLedgerRowByID :one
SELECT
	*
FROM
	credential_ledger
WHERE
	id = $1;

-- name: GetCredentialPasswordByID :one
SELECT
	*
FROM
	credential_password
WHERE
	id = $1;

-- name: GetCredentialLifecycleJournalEntriesBySubject :many
-- Entries about one credential, ordered as they were made. This machine has no
-- cycle, so one subject's entries are bounded by the sequences it allows and
-- the limit only caps what a caller will take. Callers pass one more than they
-- will accept, so receiving it tells them the set was larger.
--
-- The subject is on the line, so this joins. An entry acting on two credentials
-- is returned to each of them, once, carrying the line that concerns the one
-- asked about: what the other line did is that credential's business and is
-- reached by asking about it. The entry identifier is what shows two answers
-- came from one event.
SELECT
	j.entry_id,
	j.recording_date,
	j.effective_date,
	j.actor_type,
	j.actor,
	j.entailed_by_entry,
	j.entailed_by_annotation,
	l.line,
	l.subject,
	l.event
FROM
	credential_lifecycle_journal AS j
	INNER JOIN credential_lifecycle_journal_line AS l ON l.entry_id = j.entry_id
WHERE
	l.subject = $1
ORDER BY
	j.entry_id
LIMIT
	$2;

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
	credential_ledger
WHERE
	holder_type = $1
	AND holder_id = $2
	AND state = 'valid';

-- name: InvalidateCredential :one
-- Post a credential to `invalid`. Two transitions reach that state, `revoke`
-- and `lapse`, so this is named for the posting rather than for either of
-- them. Which one occurred is the entry's business.
--
-- Conditional on the posting reference the caller last saw, so that two posters
-- cannot both believe they succeeded.
UPDATE
	credential_ledger
SET
	state = 'invalid',
	lifecycle_posting_reference = $2
WHERE
	id = $1
	AND lifecycle_posting_reference = $3 RETURNING *;

-- name: InsertCredentialAPIKey :one
-- The api_key type's own state, keyed on the ledger row it belongs to and
-- written in the same transaction as it.
INSERT INTO
	credential_api_key (id, key_id, hashed_secret, token_name, scopes, allow_list)
VALUES
	($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: GetCredentialAPIKeyByID :one
SELECT
	*
FROM
	credential_api_key
WHERE
	id = $1;

-- name: GetCredentialAPIKeyByKeyID :one
-- Resolve the public half of a token into the credential it names. This is how
-- a presentation arriving over the wire finds its subject, the wire carrying a
-- key id where the model carries an identifier.
SELECT
	*
FROM
	credential_api_key
WHERE
	key_id = $1;

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

-- name: NextCredentialUseJournalEntryID :one
-- One call per entry. The use journal has no line table, neither of its
-- operations taking parameters.
SELECT
	nextval('credential_use_journal_entry_seq')::bigint;

-- name: InsertCredentialUseJournalEntry :one
-- recording_date is absent on purpose: the column default supplies it, so no
-- caller can supply, override, or backdate it. The annotation column is not
-- read by posting, which is what its name promises.
INSERT INTO
	credential_use_journal (
		entry_id,
		effective_date,
		actor_type,
		actor,
		event,
		subject,
		annotation_source
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7) RETURNING *;

-- name: PostCredentialPresentation :one
-- Posting an assignment to the use model's variables.
--
-- Conditional on the entry being newer than whatever last posted, which is how
-- "post in journal order" is enforced for a variable. An older entry arriving
-- late affects no rows, and that is correct rather than a failure: the fold in
-- journal order would give the newer value anyway.
--
-- last_used is assigned only when the presentation was accepted, which the
-- caller states rather than the statement inferring.
UPDATE
	credential_ledger
SET
	last_presented = @presented_at,
	last_used = CASE WHEN @accepted::boolean THEN @presented_at ELSE last_used END,
	use_posting_reference = @entry_id
WHERE
	id = @id
	AND (use_posting_reference IS NULL OR use_posting_reference < @entry_id) RETURNING *;

-- name: GetCredentialUseJournalEntriesBySubject :many
-- Entries about one credential's use, in journal order. Unbounded in principle:
-- a variable takes assignments without limit, so the caller's limit is a cap it
-- chooses rather than one the model guarantees.
SELECT
	*
FROM
	credential_use_journal
WHERE
	subject = $1
ORDER BY
	entry_id
LIMIT
	$2;
