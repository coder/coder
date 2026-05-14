ALTER TABLE template_version_dlp_policies
DROP CONSTRAINT IF EXISTS template_version_dlp_policies_template_version_id_key;

ALTER TABLE template_version_dlp_policies
ADD CONSTRAINT template_version_dlp_policies_template_version_id_name_key
UNIQUE (template_version_id, name);

ALTER TABLE workspace_agents
ADD COLUMN dlp_policy_id UUID NULL;

ALTER TABLE workspace_agents
ADD CONSTRAINT workspace_agents_dlp_policy_id_fkey
FOREIGN KEY (dlp_policy_id)
REFERENCES template_version_dlp_policies (id)
ON DELETE SET NULL;
