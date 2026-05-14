-- The DLP policy is now declared once per template and applies to every
-- agent in the resulting workspace, so the per-agent FK is no longer used.
ALTER TABLE workspace_agents
DROP CONSTRAINT IF EXISTS workspace_agents_dlp_policy_id_fkey;

ALTER TABLE workspace_agents
DROP COLUMN IF EXISTS dlp_policy_id;

-- Tighten cardinality: at most one coder_dlp_policy per template version.
-- The name column is retained because denial reasons surface it in
-- connection logs (e.g. `DLP policy "strict" denied ssh_access`).
ALTER TABLE template_version_dlp_policies
DROP CONSTRAINT IF EXISTS template_version_dlp_policies_template_version_id_name_key;

ALTER TABLE template_version_dlp_policies
ADD CONSTRAINT template_version_dlp_policies_template_version_id_key
UNIQUE (template_version_id);
