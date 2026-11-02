-- GetTemplateAppInsights finds the newest app with a slug by walking this
-- index newest first, and rejects usage names that are no app's slug with a
-- single probe, instead of joining every app in the deployment.
CREATE INDEX workspace_apps_slug_created_at_idx ON workspace_apps (slug, created_at);
