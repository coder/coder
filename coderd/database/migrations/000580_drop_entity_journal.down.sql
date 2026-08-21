CREATE TABLE entity_journal (
	id BIGSERIAL PRIMARY KEY,
	recorded_at timestamptz NOT NULL,
	event text NOT NULL,
	subject_type text NOT NULL,
	subject uuid NOT NULL,
	actor_type text NOT NULL,
	actor uuid NOT NULL
);

CREATE INDEX idx_entity_journal_subject ON entity_journal (subject_type, subject);

CREATE INDEX idx_entity_journal_actor ON entity_journal (actor_type, actor);

CREATE TABLE entity_ai_agents (
	id uuid PRIMARY KEY,
	owner_id uuid NOT NULL REFERENCES users (id)
);
