CREATE TABLE ai_sandboxes (
	id uuid NOT NULL PRIMARY KEY,
	workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
	-- The parent agent that owns this sandbox's lifecycle: it runs the
	-- create and destroy scripts and the egress proxy.
	parent_agent_id uuid NOT NULL REFERENCES workspace_agents (id) ON DELETE CASCADE,
	-- The confined child agent running inside the sandbox.
	child_agent_id uuid NOT NULL REFERENCES workspace_agents (id) ON DELETE CASCADE,
	-- The AI identity bound to the child. Deliberately no cascade:
	-- identity revocation is a soft delete and must not remove lifecycle
	-- records, matching workspace_agents.ai_agent_id.
	ai_agent_id uuid NOT NULL REFERENCES ai_agents (user_id),
	-- Declaration key used to reconcile after a parent agent restart.
	name text NOT NULL,
	egress_enforcement text NOT NULL CHECK (egress_enforcement IN ('forced', 'advisory', 'none')),
	created_at timestamptz NOT NULL DEFAULT now(),
	deleted boolean NOT NULL DEFAULT false
);

COMMENT ON TABLE ai_sandboxes IS
	'Lifecycle records for AI sandboxes created by a parent workspace agent from an admin-authored script declaration. Distinct from ai_sandbox_sessions, which are egress audit records.';
COMMENT ON COLUMN ai_sandboxes.name IS
	'Declaration name, unique per parent agent while not deleted, so a restarted parent reconciles to the existing sandbox instead of creating a duplicate.';
COMMENT ON COLUMN ai_sandboxes.egress_enforcement IS
	'Admin attestation of routing coverage declared for this sandbox. Recorded, not verified.';

CREATE UNIQUE INDEX idx_ai_sandboxes_parent_name ON ai_sandboxes (parent_agent_id, name) WHERE NOT deleted;
CREATE INDEX idx_ai_sandboxes_child_agent_id ON ai_sandboxes (child_agent_id);
CREATE INDEX idx_ai_sandboxes_workspace_id ON ai_sandboxes (workspace_id);
