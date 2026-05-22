-- Per-user user_secrets caps (count, total stored bytes, env-injected
-- stored bytes), enforced at the schema level.
--
-- Why: user_secrets is user-scoped; every workspace loads the same
-- set via the agent manifest, and env-injected ones land in the
-- agent's process env. Without a cap the failure surfaces at
-- workspace start (or as a truncated env), not at create-time.
--
-- What drives each cap:
--
--   * count_limit = 50: backstop, well under the ~170-secret ceiling
--     where the manifest would overflow at 24 KiB per value
--     (MaxUserSecretValueBytes).
--
--   * total_bytes_limit = 1 MiB: ~25 % of the 4 MiB DRPC manifest
--     budget (codersdk/drpcsdk.MaxMessageSize); the rest is for
--     apps/scripts/metadata/devcontainers.
--
--   * env_bytes_limit = 24 KiB: under the ~32 KiB Windows process
--     env block with headroom for the agent's own env (CODER_*,
--     PATH, HOME, ...). file_path secrets bypass the env block and
--     are not counted. Linux/macOS ARG_MAX (~2 MiB) is far above
--     this, so the same cap works everywhere.
--
-- octet_length(value) measures stored bytes. In encrypted
-- deployments stored bytes exceed plaintext (AES-GCM + base64
-- ~1.33x). The handler's per-value check (UserSecretValueValid)
-- measures plaintext separately, so it can pass while the
-- trigger's stored-bytes aggregate rejects. The trigger is
-- authoritative; the handler is a fast pre-flight.
--
-- Keep the literals below in sync with codersdk.MaxUserSecret*
-- in codersdk/usersecretvalidation.go. TestUserSecretLimits in
-- coderd/usersecrets_test.go exercises off-by-one for each cap,
-- so any drift between the two layers fails an assertion.
CREATE FUNCTION enforce_user_secrets_per_user_limits() RETURNS trigger
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
    total_bytes_limit constant bigint := 1048576;  -- 1 MiB
    env_bytes_limit   constant bigint := 24576;    -- 24 KiB
BEGIN
    -- Serialize cap checks per user so concurrent inserts cannot all
    -- observe the same pre-insert aggregates and exceed the cap.
    PERFORM 1 FROM users WHERE id = NEW.user_id FOR UPDATE;

    -- Sum existing rows excluding the row being updated (so UPDATE statements
    -- don't double-count NEW). On INSERT, no row matches NEW.id, so
    -- the FILTER is a no-op.
    SELECT
        count(*) FILTER (WHERE id IS DISTINCT FROM NEW.id),
        coalesce(sum(octet_length(value)) FILTER (WHERE id IS DISTINCT FROM NEW.id), 0),
        coalesce(sum(octet_length(value)) FILTER (WHERE id IS DISTINCT FROM NEW.id AND env_name <> ''), 0)
    INTO existing_count, existing_total_bytes, existing_env_bytes
    FROM user_secrets
    WHERE user_id = NEW.user_id;

    new_count       := existing_count + 1;
    new_total_bytes := existing_total_bytes + octet_length(NEW.value);
    new_env_bytes   := existing_env_bytes
                       + CASE WHEN NEW.env_name <> '' THEN octet_length(NEW.value) ELSE 0 END;

    IF new_count > count_limit THEN
        RAISE EXCEPTION 'user has reached the user secrets count limit (% > %)',
            new_count, count_limit
            USING ERRCODE = 'check_violation',
                  CONSTRAINT = 'user_secrets_per_user_count_limit';
    END IF;

    IF new_total_bytes > total_bytes_limit THEN
        RAISE EXCEPTION 'user has reached the user secrets total value bytes limit (% > %)',
            new_total_bytes, total_bytes_limit
            USING ERRCODE = 'check_violation',
                  CONSTRAINT = 'user_secrets_per_user_total_bytes_limit';
    END IF;

    IF new_env_bytes > env_bytes_limit THEN
        RAISE EXCEPTION 'user has reached the env-injected user secrets bytes limit (% > %)',
            new_env_bytes, env_bytes_limit
            USING ERRCODE = 'check_violation',
                  CONSTRAINT = 'user_secrets_per_user_env_bytes_limit';
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER trigger_user_secrets_per_user_limits
    BEFORE INSERT OR UPDATE ON user_secrets
    FOR EACH ROW
EXECUTE PROCEDURE enforce_user_secrets_per_user_limits();
