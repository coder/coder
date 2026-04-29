CREATE TABLE template_version_dlp_policies
(
	id UUID PRIMARY KEY NOT NULL,
	template_version_id UUID NOT NULL,
	name TEXT NOT NULL,
	ssh_access BOOLEAN NOT NULL,
	web_terminal_access BOOLEAN NOT NULL,
	port_forwarding_access BOOLEAN NOT NULL,
	allowed_applications TEXT[],
	created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (template_version_id) REFERENCES template_versions (id) ON DELETE CASCADE,
	UNIQUE (template_version_id, name)
);

ALTER TABLE workspace_agents
ADD COLUMN dlp_policy_id UUID NULL;

ALTER TABLE workspace_agents
ADD CONSTRAINT workspace_agents_dlp_policy_id_fkey
FOREIGN KEY (dlp_policy_id)
REFERENCES template_version_dlp_policies (id)
ON DELETE SET NULL;
