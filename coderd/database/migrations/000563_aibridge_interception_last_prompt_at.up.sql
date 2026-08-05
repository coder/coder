ALTER TABLE aibridge_interceptions
	ADD COLUMN last_prompt_at timestamp with time zone;

COMMENT ON COLUMN aibridge_interceptions.last_prompt_at IS 'Denormalized cache of the maximum aibridge_user_prompts.created_at recorded for this interception. NULL when the interception has no user prompts. Maintained monotonically on prompt insert and used to sort sessions by last activity without joining the prompts table.';

-- Backfill existing rows from the prompts table.
UPDATE aibridge_interceptions ai
	SET last_prompt_at = up.max_created_at
	FROM (
		SELECT interception_id, MAX(created_at) AS max_created_at
		FROM aibridge_user_prompts
		GROUP BY interception_id
	) up
	WHERE up.interception_id = ai.id;
