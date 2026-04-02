ALTER TABLE aibridge_interceptions
    DROP COLUMN IF EXISTS credential_kind,
    DROP COLUMN IF EXISTS credential_hint;
