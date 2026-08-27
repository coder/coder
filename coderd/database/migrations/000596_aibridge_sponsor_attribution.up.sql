ALTER TABLE aibridge_interceptions
	ADD COLUMN sponsor_user_id uuid;

COMMENT ON COLUMN aibridge_interceptions.sponsor_user_id IS
	'Sponsoring human user for requests initiated by an AI identity. Not a foreign key; audit history survives user cleanup like the sandbox and escalation snapshots.';
