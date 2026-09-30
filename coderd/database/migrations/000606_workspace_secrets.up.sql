-- Workspace-scoped secrets set inline on a workspace build request and
-- delivered to the workspace only through the agent manifest. Unlike rich
-- parameters they are never passed to the provisioner, so they do not
-- appear in workspace_build_parameters or Terraform state. Rows are
-- carried forward across builds until replaced or removed.
--
-- The value, value_key_id, env_name, and file_path columns mirror
-- user_secrets so the same dbcrypt and agent manifest code paths apply.
CREATE TABLE workspace_secrets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    -- The encrypted secret value (base64-encoded encrypted data).
    value TEXT NOT NULL,
    value_key_id TEXT REFERENCES dbcrypt_keys(active_key_digest),
    -- Environment variable name. Empty string means don't inject as env var.
    env_name TEXT NOT NULL DEFAULT '',
    -- File path the secret is written to. Empty string means don't write a file.
    file_path TEXT NOT NULL DEFAULT '',
    -- The build whose request last set this value. Informational only:
    -- every later build of the workspace receives the row as well.
    updated_by_build_id UUID NOT NULL REFERENCES workspace_builds(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP NOT NULL,
    -- Workspace secrets have no enabled flag, so every row must have an
    -- injection target.
    CONSTRAINT workspace_secrets_requires_target CHECK (env_name <> '' OR file_path <> '')
);

CREATE UNIQUE INDEX workspace_secrets_workspace_name_idx ON workspace_secrets(workspace_id, name);

CREATE UNIQUE INDEX workspace_secrets_workspace_env_name_idx ON workspace_secrets(workspace_id, env_name)
WHERE env_name != '';

CREATE UNIQUE INDEX workspace_secrets_workspace_file_path_idx ON workspace_secrets(workspace_id, file_path)
WHERE file_path != '';

-- Per-workspace caps. Same limits and rationale as
-- enforce_user_secrets_per_user_limits (000509_user_secrets_limits.up.sql):
-- every row lands in the agent manifest and env-injected rows land in the
-- agent's process env. Keep the literals in sync with codersdk.MaxUserSecret*.
CREATE FUNCTION enforce_workspace_secrets_per_workspace_limits() RETURNS trigger
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
    -- Serialize cap checks per workspace so concurrent inserts cannot all
    -- observe the same pre-insert aggregates and exceed the cap.
    PERFORM 1 FROM workspaces WHERE id = NEW.workspace_id FOR UPDATE;

    SELECT
        count(*) FILTER (WHERE id IS DISTINCT FROM NEW.id),
        coalesce(sum(octet_length(value)) FILTER (WHERE id IS DISTINCT FROM NEW.id), 0),
        coalesce(sum(octet_length(value)) FILTER (WHERE id IS DISTINCT FROM NEW.id AND env_name <> ''), 0)
    INTO existing_count, existing_total_bytes, existing_env_bytes
    FROM workspace_secrets
    WHERE workspace_id = NEW.workspace_id;

    new_count       := existing_count + 1;
    new_total_bytes := existing_total_bytes + octet_length(NEW.value);
    new_env_bytes   := existing_env_bytes
                       + CASE WHEN NEW.env_name <> '' THEN octet_length(NEW.value) ELSE 0 END;

    IF new_count > count_limit THEN
        RAISE EXCEPTION 'workspace has reached the workspace secrets count limit (% > %)',
            new_count, count_limit
            USING ERRCODE = 'check_violation',
                  CONSTRAINT = 'workspace_secrets_per_workspace_count_limit';
    END IF;

    IF new_total_bytes > total_bytes_limit THEN
        RAISE EXCEPTION 'workspace has reached the workspace secrets total value bytes limit (% > %)',
            new_total_bytes, total_bytes_limit
            USING ERRCODE = 'check_violation',
                  CONSTRAINT = 'workspace_secrets_per_workspace_total_bytes_limit';
    END IF;

    IF new_env_bytes > env_bytes_limit THEN
        RAISE EXCEPTION 'workspace has reached the workspace secrets env value bytes limit (% > %)',
            new_env_bytes, env_bytes_limit
            USING ERRCODE = 'check_violation',
                  CONSTRAINT = 'workspace_secrets_per_workspace_env_bytes_limit';
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER trigger_workspace_secrets_per_workspace_limits
    BEFORE INSERT OR UPDATE ON workspace_secrets
    FOR EACH ROW
    EXECUTE FUNCTION enforce_workspace_secrets_per_workspace_limits();
