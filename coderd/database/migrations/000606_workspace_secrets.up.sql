-- Workspace secrets are set inline on a workspace build request and
-- delivered to the workspace only through the agent manifest. Unlike rich
-- parameters they are never passed to the provisioner, so they do not
-- appear in workspace_build_parameters or Terraform state.
--
-- Each row belongs to exactly one build. When a new build is created, the
-- previous build's live, non-ephemeral rows are copied to the new build and
-- the previous build's rows are cleared: value and value_key_id are set to
-- NULL and cleared_at records when. Cleared rows are kept so the history of
-- which secrets each build received remains inspectable without the values.
--
-- The value, value_key_id, env_name, and file_path columns mirror
-- user_secrets so the same dbcrypt and agent manifest code paths apply.
CREATE TABLE workspace_secrets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    workspace_build_id UUID NOT NULL REFERENCES workspace_builds(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    -- The encrypted secret value (base64-encoded encrypted data). NULL once
    -- the row has been cleared.
    value TEXT,
    value_key_id TEXT REFERENCES dbcrypt_keys(active_key_digest),
    -- Environment variable name. Empty string means don't inject as env var.
    env_name TEXT NOT NULL DEFAULT '',
    -- File path the secret is written to. Empty string means don't write a file.
    file_path TEXT NOT NULL DEFAULT '',
    -- Ephemeral secrets are delivered to the build they were set on and are
    -- not copied forward to the next build.
    ephemeral BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP NOT NULL,
    -- Set when a later build superseded this row and its value was dropped.
    cleared_at TIMESTAMP WITH TIME ZONE,
    -- Workspace secrets have no enabled flag, so every row must have an
    -- injection target.
    CONSTRAINT workspace_secrets_requires_target CHECK (env_name <> '' OR file_path <> ''),
    -- A row either holds a value or has been cleared, never both or neither.
    CONSTRAINT workspace_secrets_value_cleared CHECK ((value IS NULL) = (cleared_at IS NOT NULL))
);

CREATE UNIQUE INDEX workspace_secrets_build_name_idx ON workspace_secrets(workspace_build_id, name);

CREATE UNIQUE INDEX workspace_secrets_build_env_name_idx ON workspace_secrets(workspace_build_id, env_name)
WHERE env_name != '';

CREATE UNIQUE INDEX workspace_secrets_build_file_path_idx ON workspace_secrets(workspace_build_id, file_path)
WHERE file_path != '';

-- Live rows are looked up by workspace when assembling the agent manifest
-- and when copying forward to the next build.
CREATE INDEX workspace_secrets_workspace_live_idx ON workspace_secrets(workspace_id)
WHERE cleared_at IS NULL;

-- Per-build caps. Same limits and rationale as
-- enforce_user_secrets_per_user_limits (000509_user_secrets_limits.up.sql):
-- every live row lands in the agent manifest and env-injected rows land in
-- the agent's process env. Keep the literals in sync with
-- codersdk.MaxUserSecret*.
CREATE FUNCTION enforce_workspace_secrets_per_build_limits() RETURNS trigger
    LANGUAGE plpgsql
AS $$
DECLARE
    existing_count       int;
    existing_total_bytes bigint;
    existing_env_bytes   bigint;

    new_count       int;
    new_total_bytes bigint;
    new_env_bytes   bigint;

    count_limit       constant int    := 50;
    total_bytes_limit constant bigint := 204800;   -- 200 KiB
    env_bytes_limit   constant bigint := 24576;    -- 24 KiB
BEGIN
    -- Serialize cap checks per build so concurrent inserts cannot all
    -- observe the same pre-insert aggregates and exceed the cap.
    PERFORM 1 FROM workspace_builds WHERE id = NEW.workspace_build_id FOR UPDATE;

    SELECT
        count(*),
        coalesce(sum(octet_length(value)), 0),
        coalesce(sum(octet_length(value)) FILTER (WHERE env_name <> ''), 0)
    INTO existing_count, existing_total_bytes, existing_env_bytes
    FROM workspace_secrets
    WHERE workspace_build_id = NEW.workspace_build_id
      AND cleared_at IS NULL;

    new_count       := existing_count + 1;
    new_total_bytes := existing_total_bytes + octet_length(NEW.value);
    new_env_bytes   := existing_env_bytes
                       + CASE WHEN NEW.env_name <> '' THEN octet_length(NEW.value) ELSE 0 END;

    IF new_count > count_limit THEN
        RAISE EXCEPTION 'workspace build has reached the workspace secrets count limit (% > %)',
            new_count, count_limit
            USING ERRCODE = 'check_violation',
                  CONSTRAINT = 'workspace_secrets_per_build_count_limit';
    END IF;

    IF new_total_bytes > total_bytes_limit THEN
        RAISE EXCEPTION 'workspace build has reached the workspace secrets total value bytes limit (% > %)',
            new_total_bytes, total_bytes_limit
            USING ERRCODE = 'check_violation',
                  CONSTRAINT = 'workspace_secrets_per_build_total_bytes_limit';
    END IF;

    IF new_env_bytes > env_bytes_limit THEN
        RAISE EXCEPTION 'workspace build has reached the workspace secrets env value bytes limit (% > %)',
            new_env_bytes, env_bytes_limit
            USING ERRCODE = 'check_violation',
                  CONSTRAINT = 'workspace_secrets_per_build_env_bytes_limit';
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER trigger_workspace_secrets_per_build_limits
    BEFORE INSERT ON workspace_secrets
    FOR EACH ROW
    EXECUTE FUNCTION enforce_workspace_secrets_per_build_limits();
