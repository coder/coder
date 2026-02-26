DROP INDEX IF EXISTS idx_aibridge_interceptions_thread_root_id;

ALTER TABLE aibridge_interceptions
DROP COLUMN thread_root_id;
