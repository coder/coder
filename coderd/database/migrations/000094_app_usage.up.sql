CREATE TABLE IF NOT EXISTS app_usage (
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    app_id uuid NOT NULL REFERENCES workspace_apps (id) ON DELETE CASCADE,
    template_id uuid NOT NULL REFERENCES templates (id) ON DELETE CASCADE,
	created_at DATE NOT NULL,
	UNIQUE (user_id, app_id, template_id, created_at)
);

-- We use created_at for DAU analysis and pruning.
CREATE INDEX idx_app_usage_created_at ON agent_stats USING btree (created_at);
