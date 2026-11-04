DROP INDEX IF EXISTS idx_aibridge_interceptions_organization_id;

ALTER TABLE aibridge_interceptions
    DROP COLUMN IF EXISTS organization_source,
    DROP COLUMN IF EXISTS organization_id;
DROP TYPE IF EXISTS aibridge_interception_organization_source;
