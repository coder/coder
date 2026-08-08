ALTER TABLE
    workspace_resources
ADD
    COLUMN module_path TEXT;

CREATE TABLE workspace_modules (
    id uuid NOT NULL,
    job_id uuid NOT NULL REFERENCES provisioner_jobs (id) ON DELETE CASCADE,
    transition workspace_transition NOT NULL,
    source TEXT NOT NULL,
    version TEXT NOT NULL,
    key TEXT NOT NULL,
    created_at timestamp with time zone NOT NULL
);

CREATE INDEX workspace_modules_created_at_idx ON workspace_modules (created_at);
