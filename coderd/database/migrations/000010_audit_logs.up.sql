CREATE TYPE resource_type AS ENUM (
    'organization',
    'template',
    'template_version',
    'user',
    'workspace'
);

CREATE TYPE audit_action AS ENUM (
    'create',
    -- We intentionally do not track reads. They're way too spammy.
    'write',
    'delete'
);

CREATE TABLE audit_logs (
    id uuid NOT NULL,
    "time" timestamp with time zone NOT NULL,
    user_id uuid NOT NULL,
    organization_id uuid NOT NULL,
    ip inet NOT NULL,
    user_agent varchar(256) NOT NULL,
    resource_type resource_type NOT NULL,
    resource_id uuid NOT NULL,
    -- resource_target is the name of the resource that `resource_id` points to.
    -- it's stored here because resources we point to can be deleted.
    resource_target text NOT NULL,
    action audit_action NOT NULL,
    diff jsonb NOT NULL,
    status_code integer NOT NULL,
    PRIMARY KEY (id)
);

CREATE INDEX idx_audit_logs_time_desc ON audit_logs USING btree ("time" DESC);
CREATE INDEX idx_audit_log_user_id ON audit_logs USING btree (user_id);
CREATE INDEX idx_audit_log_organization_id ON audit_logs USING btree (organization_id);
CREATE INDEX idx_audit_log_resource_id ON audit_logs USING btree (resource_id);
