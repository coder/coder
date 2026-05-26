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
--   * count_limit = 50: backstop against row-count growth from many
--     small secrets. The total_bytes_limit binds first for large
--     secrets; this binds first for typical-sized ones (~few KB).
--
--   * total_bytes_limit = 200 KiB: sized to cover realistic
--     credential storage (API keys, SSH keys, kubeconfigs, cert
--     bundles) with headroom. Well under the 4 MiB DRPC manifest
--     budget (codersdk/drpcsdk.MaxMessageSize).
--
--   * env_bytes_limit = 24 KiB: an approximate budget for the
--     value bytes of env-injected secrets. Leaves ~8 KiB of
--     headroom under the ~32 KiB Windows process env block
--     (CreateProcessW's lpEnvironment is capped at 32,767
--     characters) for what this aggregate does not count:
--     env_name bytes, per-entry overhead, agent-injected vars
--     (CODER_*, PATH, HOME, ...), and template-defined env. Not
--     a strict overflow guarantee. Linux/macOS ARG_MAX (~2 MiB)
--     is far above this, so the same cap works everywhere.
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
    total_bytes_limit constant bigint := 204800;   -- 200 KiB
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
