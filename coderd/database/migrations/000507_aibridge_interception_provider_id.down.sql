DROP INDEX IF EXISTS idx_aibridge_interceptions_provider_id;

ALTER TABLE aibridge_interceptions
    DROP COLUMN IF EXISTS provider_id;
