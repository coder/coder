CREATE TYPE ai_agent_origin AS ENUM ('chat', 'workspace');

CREATE TABLE ai_agents (
    user_id uuid NOT NULL,
    owner_user_id uuid NOT NULL,
    origin_type ai_agent_origin NOT NULL,
    origin_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted boolean DEFAULT false NOT NULL,
    CONSTRAINT ai_agents_pkey PRIMARY KEY (user_id),
    CONSTRAINT ai_agents_user_id_fkey FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT ai_agents_owner_user_id_fkey FOREIGN KEY (owner_user_id) REFERENCES users (id) ON DELETE CASCADE
);

CREATE INDEX idx_ai_agents_owner ON ai_agents USING btree (owner_user_id);

-- The rows are rebuilt from the ledger. This is not a reconstruction of lost
-- history: the table was a projection of ai_agent_ledger and every column it
-- held is still there, so the projection can simply be taken again. Migrations
-- below this one repoint foreign keys back at this table and would fail on
-- rows that are still referenced.
INSERT INTO ai_agents (user_id, owner_user_id, origin_type, origin_id, created_at, deleted)
SELECT
    id,
    owner_id,
    creation_site_type::ai_agent_origin,
    creation_site_id,
    creation_time,
    state <> 'active'
FROM ai_agent_ledger;
