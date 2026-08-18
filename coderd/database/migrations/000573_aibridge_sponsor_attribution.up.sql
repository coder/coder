ALTER TABLE aibridge_interceptions
	ADD COLUMN sponsor_user_id uuid REFERENCES users(id);

COMMENT ON COLUMN aibridge_interceptions.sponsor_user_id IS
	'Sponsoring human user for requests initiated by an AI identity.';
