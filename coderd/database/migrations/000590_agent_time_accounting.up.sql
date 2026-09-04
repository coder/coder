CREATE TABLE agent_time_capture (
    id smallint PRIMARY KEY DEFAULT 1,
    capture_started_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT agent_time_capture_singleton_check CHECK (id = 1)
);

INSERT INTO agent_time_capture (id, capture_started_at)
VALUES (1, now());

CREATE TABLE agent_time_daily (
    organization_id uuid NOT NULL,
    user_id uuid NOT NULL,
    day date NOT NULL,
    agent_time_ms bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (organization_id, user_id, day)
);

CREATE INDEX agent_time_daily_organization_day_user_id_idx
    ON agent_time_daily (organization_id, day, user_id);

CREATE INDEX agent_time_daily_day_organization_user_id_idx
    ON agent_time_daily (day, organization_id, user_id);

-- Overview summaries avoid scanning every user's daily history. They are
-- maintained from the same newly claimed contributions as the canonical rows.
CREATE TABLE agent_time_organization_daily (
    organization_id uuid NOT NULL,
    day date NOT NULL,
    agent_time_ms numeric NOT NULL DEFAULT 0,
    PRIMARY KEY (organization_id, day)
);

CREATE INDEX agent_time_organization_daily_day_idx
    ON agent_time_organization_daily (day, organization_id);

CREATE INDEX agent_time_daily_user_day_idx
    ON agent_time_daily (user_id, day, organization_id);

CREATE TABLE chat_message_agent_time_accounted (
    message_id bigint PRIMARY KEY REFERENCES chat_messages(id) ON DELETE CASCADE,
    organization_id uuid NOT NULL
);

CREATE INDEX chat_message_agent_time_accounted_organization_id_idx
    ON chat_message_agent_time_accounted (organization_id);

CREATE INDEX idx_chat_messages_agent_time_lookup
    ON chat_messages (chat_id, id)
    WHERE runtime_ms IS NOT NULL;

CREATE TABLE agent_time_backfill_status (
    organization_id uuid PRIMARY KEY,
    cursor_message_id bigint NOT NULL DEFAULT 0,
    processed_messages bigint NOT NULL DEFAULT 0,
    completed_at timestamptz,
    last_error text NOT NULL DEFAULT '',
    last_error_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT agent_time_backfill_status_cursor_message_id_nonnegative CHECK (cursor_message_id >= 0),
    CONSTRAINT agent_time_backfill_status_processed_messages_nonnegative CHECK (processed_messages >= 0)
);

CREATE FUNCTION agent_time_delete_fallback_limit()
RETURNS integer
LANGUAGE sql
IMMUTABLE
AS $$
    SELECT 10000;
$$;

-- Supported writers set recorded time, creation date, and attribution on insert.
-- Do not update messages for accounting: chatstate treats message updates as
-- history revisions. Late changes to these immutable inputs require explicit
-- accounting support before introducing such a writer.
CREATE FUNCTION account_agent_time_messages(
    p_message_ids bigint[],
    p_delete_chat_id uuid DEFAULT NULL,
    p_delete_organization_id uuid DEFAULT NULL,
    p_delete_user_id uuid DEFAULT NULL
)
RETURNS bigint
LANGUAGE sql
AS $$
WITH input_messages AS MATERIALIZED (
    SELECT DISTINCT message_id
    FROM unnest(COALESCE(p_message_ids, ARRAY[]::bigint[])) AS input(message_id)
    WHERE message_id IS NOT NULL
    ORDER BY message_id
),
locked_chats AS MATERIALIZED (
    SELECT c.id
    FROM chats c
    JOIN (
        SELECT DISTINCT cm.chat_id
        FROM chat_messages cm
        JOIN input_messages input ON input.message_id = cm.id
        WHERE p_delete_chat_id IS NULL
    ) target_chats ON target_chats.chat_id = c.id
    ORDER BY c.id
    -- Inserts already hold FK KEY SHARE locks. NO KEY UPDATE serializes
    -- accounting without conflicting with another insert's FK lock.
    FOR NO KEY UPDATE OF c
),
candidate_messages AS MATERIALIZED (
    SELECT
        cm.id AS message_id,
        COALESCE(p_delete_organization_id, c.organization_id) AS organization_id,
        COALESCE(p_delete_user_id, c.owner_id) AS user_id,
        (cm.created_at AT TIME ZONE 'UTC')::date AS day,
        cm.runtime_ms
    FROM input_messages input
    JOIN chat_messages cm ON cm.id = input.message_id
    LEFT JOIN chats c ON c.id = cm.chat_id AND p_delete_chat_id IS NULL
    WHERE cm.runtime_ms IS NOT NULL
      AND (
          (p_delete_chat_id IS NULL AND c.id IN (SELECT id FROM locked_chats))
          OR (p_delete_chat_id IS NOT NULL AND cm.chat_id = p_delete_chat_id)
      )
      AND COALESCE(p_delete_organization_id, c.organization_id) IS NOT NULL
      AND COALESCE(p_delete_user_id, c.owner_id) IS NOT NULL
),
claimed_messages AS MATERIALIZED (
    INSERT INTO chat_message_agent_time_accounted (message_id, organization_id)
    SELECT message_id, organization_id
    FROM candidate_messages
    ORDER BY message_id
    ON CONFLICT (message_id) DO NOTHING
    RETURNING message_id
),
increments AS MATERIALIZED (
    SELECT
        candidate_messages.organization_id,
        candidate_messages.user_id,
        candidate_messages.day,
        SUM(candidate_messages.runtime_ms)::bigint AS agent_time_ms
    FROM candidate_messages
    JOIN claimed_messages ON claimed_messages.message_id = candidate_messages.message_id
    GROUP BY candidate_messages.organization_id, candidate_messages.user_id, candidate_messages.day
),
updated_daily AS (
    INSERT INTO agent_time_daily (organization_id, user_id, day, agent_time_ms)
    SELECT organization_id, user_id, day, agent_time_ms
    FROM increments
    ORDER BY organization_id, user_id, day
    ON CONFLICT (organization_id, user_id, day) DO UPDATE SET
        agent_time_ms = agent_time_daily.agent_time_ms + EXCLUDED.agent_time_ms
    RETURNING 1
),
updated_daily_count AS MATERIALIZED (
    SELECT COUNT(*) AS rows_updated FROM updated_daily
),
updated_organization_daily AS (
    INSERT INTO agent_time_organization_daily (organization_id, day, agent_time_ms)
    SELECT organization_id, day, SUM(agent_time_ms)
    FROM increments
    CROSS JOIN updated_daily_count
    GROUP BY organization_id, day
    ORDER BY organization_id, day
    ON CONFLICT (organization_id, day) DO UPDATE SET
        agent_time_ms = agent_time_organization_daily.agent_time_ms + EXCLUDED.agent_time_ms
    RETURNING 1
),
updated_organization_count AS (
    SELECT COUNT(*) AS rows_updated FROM updated_organization_daily
)
SELECT COUNT(*)::bigint
FROM claimed_messages, updated_organization_count;
$$;

CREATE FUNCTION agent_time_account_chat_messages_after_insert()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    PERFORM account_agent_time_messages(ARRAY(
        SELECT id
        FROM agent_time_new_messages
        WHERE runtime_ms IS NOT NULL
        ORDER BY id
    ));
    RETURN NULL;
END;
$$;

CREATE TRIGGER trigger_agent_time_account_chat_messages_after_insert
AFTER INSERT ON chat_messages
REFERENCING NEW TABLE AS agent_time_new_messages
FOR EACH STATEMENT
EXECUTE FUNCTION agent_time_account_chat_messages_after_insert();

CREATE FUNCTION agent_time_preserve_chat_before_delete()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    message_ids bigint[];
    unaccounted_count bigint;
    fallback_count bigint;
    fallback_limit integer;
BEGIN
    fallback_limit := agent_time_delete_fallback_limit();
    fallback_count := COALESCE(NULLIF(current_setting('coder.agent_time_delete_fallback_count', true), '')::bigint, 0);

    SELECT
        COALESCE(array_agg(pending.message_id ORDER BY pending.message_id), ARRAY[]::bigint[]),
        COUNT(*)::bigint
    INTO message_ids, unaccounted_count
    FROM (
        SELECT cm.id AS message_id
        FROM chat_messages cm
        LEFT JOIN chat_message_agent_time_accounted accounted ON accounted.message_id = cm.id
        WHERE cm.chat_id = OLD.id
          AND cm.runtime_ms IS NOT NULL
          AND accounted.message_id IS NULL
        ORDER BY cm.id
        LIMIT fallback_limit + 1
    ) pending;

    IF fallback_count + unaccounted_count > fallback_limit THEN
        RAISE EXCEPTION 'chat deletion would preserve % unaccounted agent time messages, exceeding fallback limit %',
            fallback_count + unaccounted_count, fallback_limit;
    END IF;

    PERFORM set_config('coder.agent_time_delete_fallback_count', (fallback_count + unaccounted_count)::text, true);
    PERFORM account_agent_time_messages(message_ids, OLD.id, OLD.organization_id, OLD.owner_id);
    RETURN OLD;
END;
$$;

CREATE TRIGGER trigger_agent_time_preserve_chat_before_delete
BEFORE DELETE ON chats
FOR EACH ROW
EXECUTE FUNCTION agent_time_preserve_chat_before_delete();
