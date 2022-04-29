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
    ip cidr NOT NULL,
    os varchar(64) NOT NULL,
    browser varchar(64) NOT NULL,
    device varchar(64) NOT NULL,
    resource_type resource_type NOT NULL,
    resource_id uuid NOT NULL,
    resource_target text NOT NULL,
    action audit_action NOT NULL,
    diff jsonb NOT NULL,
    status_code integer DEFAULT 0 NOT NULL,
    PRIMARY KEY (id)
);

CREATE INDEX idx_audit_logs_time_desc ON audit_logs USING btree ("time" DESC);
CREATE INDEX idx_audit_log_user_id ON audit_logs USING btree (user_id);
CREATE INDEX idx_audit_log_organization_id ON audit_logs USING btree (organization_id);
CREATE INDEX idx_audit_log_resource_id ON audit_logs USING btree (resource_id);

-- count_estimate returns the number of rows the query planner estimates the
-- provided query will return.
CREATE FUNCTION count_estimate(query text)
  RETURNS integer
  LANGUAGE plpgsql AS
$func$
DECLARE
    rec   record;
    rows  integer;
BEGIN
    FOR rec IN EXECUTE 'EXPLAIN ' || query LOOP
        rows := substring(rec."QUERY PLAN" FROM ' rows=([[:digit:]]+)');
        EXIT WHEN rows IS NOT NULL;
    END LOOP;

    RETURN rows;
END
$func$;
