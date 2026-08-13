CREATE TABLE ai_agents (
	id uuid PRIMARY KEY,
	-- The principal on whose behalf the AI agent acts.
	owner_id uuid NOT NULL REFERENCES users (id)
);

COMMENT ON TABLE ai_agents IS 'Identities of AI agents. Three absences are deliberate. There is no workspace or sandbox reference, because an AI agent''s identity is independent of where it runs and may outlive any particular sandbox. There is no execution state, because an identity and a run of it are different things, and a schema that merges them forecloses reconstituting an AI agent from a previous session. There is no creation time, because the journal records when this row came to exist and duplicating it here would create a second answer that can disagree with the first.';
