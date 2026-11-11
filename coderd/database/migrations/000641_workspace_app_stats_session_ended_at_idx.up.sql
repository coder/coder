CREATE INDEX workspace_app_stats_session_ended_at_idx ON workspace_app_stats USING btree (session_ended_at);

COMMENT ON INDEX workspace_app_stats_session_ended_at_idx IS 'Supports the session_ended_at range filter in GetTemplateAppInsightsByTemplate (insights metrics collector).';
