CREATE INDEX workspace_agent_stats_template_id_created_at_user_id_idx ON workspace_agent_stats USING btree (template_id, created_at DESC, user_id) WHERE connection_count > 0;

COMMENT ON INDEX workspace_agent_stats_template_id_created_at_user_id_idx IS 'Support index for template insights endpoint to build interval reports faster.';
