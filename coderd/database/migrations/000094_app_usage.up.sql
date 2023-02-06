CREATE TABLE IF NOT EXISTS app_usage (
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    app_slug text NOT NULL,
    template_id uuid NOT NULL REFERENCES templates (id) ON DELETE CASCADE,
	created_at DATE NOT NULL,
	UNIQUE (user_id, app_slug, template_id, created_at)
);

-- We use created_at for DAU analysis and pruning.
CREATE INDEX idx_app_usage_created_at ON app_usage USING btree (created_at);

-- We perform app grouping to analyze DAUs.
CREATE INDEX idx_app_usage_app_slug ON app_usage USING btree (app_slug);
