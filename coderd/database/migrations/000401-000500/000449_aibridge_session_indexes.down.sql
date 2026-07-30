DROP INDEX IF EXISTS idx_aibridge_interceptions_session_id;
DROP INDEX IF EXISTS idx_aibridge_user_prompts_interception_created;
DROP INDEX IF EXISTS idx_aibridge_interceptions_sessions_filter;

ALTER TABLE aibridge_interceptions DROP COLUMN IF EXISTS session_id;
