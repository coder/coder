ALTER TABLE aibridge_interceptions
    ADD COLUMN provider_id uuid REFERENCES ai_providers(id) ON DELETE SET NULL;

COMMENT ON COLUMN aibridge_interceptions.provider_id IS 'The ai_providers row this interception was routed through. NULL for legacy rows that pre-date this column or whose provider could not be unambiguously resolved at backfill time. The provider/provider_name text columns remain a point-in-time snapshot regardless of later renames or deletions of the referenced provider.';

-- Backfill pass 1: match on provider_name (instance name), which is unique
-- across non-deleted ai_providers rows. provider_name was added in 000458;
-- older interception rows have provider_name = '' (its column default).
UPDATE aibridge_interceptions ai
SET provider_id = ap.id
FROM ai_providers ap
WHERE ai.provider_id IS NULL
    AND ai.provider_name != ''
    AND ap.deleted = FALSE
    AND ap.name = ai.provider_name;

-- Backfill pass 2: for legacy rows where provider_name is empty, fall back to
-- matching on provider type, but only when exactly one live ai_providers row
-- has that type. Otherwise the mapping is ambiguous and the row stays NULL.
WITH unambiguous AS (
    SELECT type
    FROM ai_providers
    WHERE deleted = FALSE
    GROUP BY type
    HAVING COUNT(*) = 1
)
UPDATE aibridge_interceptions ai
SET provider_id = ap.id
FROM unambiguous u
JOIN ai_providers ap ON ap.type = u.type AND ap.deleted = FALSE
WHERE ai.provider_id IS NULL
    AND ai.provider_name = ''
    AND ai.provider::ai_provider_type = u.type;

-- Index supports both the filter path and the ON DELETE SET NULL referential
-- action against ai_providers; without it, deleting a provider would
-- sequentially scan this audit-log table.
CREATE INDEX idx_aibridge_interceptions_provider_id
    ON aibridge_interceptions (provider_id);
