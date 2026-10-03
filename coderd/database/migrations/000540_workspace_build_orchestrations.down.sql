-- Enum additions to api_key_scope are intentionally not reversed
-- because Postgres cannot drop enum values safely.

DROP TABLE IF EXISTS workspace_build_orchestrations;

ALTER TABLE template_version_presets
    DROP CONSTRAINT IF EXISTS template_version_presets_id_template_version_id_key;

ALTER TABLE workspace_builds
    DROP CONSTRAINT IF EXISTS workspace_builds_id_workspace_id_key;
