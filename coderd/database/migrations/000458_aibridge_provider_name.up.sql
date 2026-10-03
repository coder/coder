ALTER TABLE aibridge_interceptions ADD COLUMN provider_name TEXT NOT NULL DEFAULT '';

COMMENT ON COLUMN aibridge_interceptions.provider_name IS 'The provider instance name which may differ from provider when multiple instances of the same provider type exist.';

-- Backfill existing records with the provider type as the provider name.
UPDATE aibridge_interceptions SET provider_name = provider WHERE provider_name = '';
