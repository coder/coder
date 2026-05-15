ALTER TABLE workspace_agents
DROP CONSTRAINT IF EXISTS workspace_agents_dlp_policy_id_fkey;

ALTER TABLE workspace_agents
DROP COLUMN IF EXISTS dlp_policy_id;

DROP TABLE IF EXISTS template_version_dlp_policies;
