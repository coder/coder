ALTER TABLE aibridge_interceptions
    DROP COLUMN IF EXISTS organization_source,
    DROP COLUMN IF EXISTS organization_id;
DROP TYPE IF EXISTS aibridge_interception_organization_source;
