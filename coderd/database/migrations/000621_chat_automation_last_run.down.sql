ALTER TABLE chat_automations
    DROP CONSTRAINT IF EXISTS chat_automations_last_run_shape,
    DROP COLUMN IF EXISTS last_run_at,
    DROP COLUMN IF EXISTS last_run_outcome,
    DROP COLUMN IF EXISTS last_run_reason,
    DROP COLUMN IF EXISTS last_run_chat_id;

DROP TYPE IF EXISTS chat_automation_run_outcome;
