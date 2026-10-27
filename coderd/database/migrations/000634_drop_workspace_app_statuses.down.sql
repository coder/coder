CREATE TYPE workspace_app_status_state AS ENUM (
    'working',
    'complete',
    'failure',
    'idle'
);

-- Dropped app status rows cannot be recovered.
CREATE TABLE workspace_app_statuses (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    agent_id uuid NOT NULL,
    app_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    state workspace_app_status_state NOT NULL,
    message text NOT NULL,
    uri text
);

ALTER TABLE ONLY workspace_app_statuses
    ADD CONSTRAINT workspace_app_statuses_pkey PRIMARY KEY (id);

CREATE INDEX idx_workspace_app_statuses_workspace_id_created_at ON workspace_app_statuses USING btree (workspace_id, created_at DESC);

CREATE INDEX workspace_app_statuses_app_id_idx ON workspace_app_statuses USING btree (app_id, created_at DESC);

ALTER TABLE ONLY workspace_app_statuses
    ADD CONSTRAINT workspace_app_statuses_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES workspace_agents(id);

ALTER TABLE ONLY workspace_app_statuses
    ADD CONSTRAINT workspace_app_statuses_app_id_fkey FOREIGN KEY (app_id) REFERENCES workspace_apps(id);

ALTER TABLE ONLY workspace_app_statuses
    ADD CONSTRAINT workspace_app_statuses_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id);
