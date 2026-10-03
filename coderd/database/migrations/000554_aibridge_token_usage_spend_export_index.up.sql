-- Serves spend queries that filter token usage by effective group over a time
-- range. Rows with no effective group are excluded.
CREATE INDEX idx_aibridge_token_usages_effective_group_id_created_at
    ON aibridge_token_usages (effective_group_id, created_at)
    WHERE effective_group_id IS NOT NULL;
