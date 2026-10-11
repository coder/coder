CREATE TABLE agent_runtime_hourly_usage (
    -- Start of the hour, equal to the created_at of its hb_agent_runtime_v1
    -- usage event.
    bucket_start    TIMESTAMPTZ NOT NULL,
    -- The chat's organization.
    organization_id UUID        NOT NULL,
    -- The owner's effective Agent Hours group when the hour was generated.
    -- Equals organization_id for the Everyone group, which holds the hours
    -- of owners without an allotted group.
    group_id        UUID        NOT NULL,
    -- The chat owner.
    user_id         UUID        NOT NULL,
    runtime_ms      BIGINT      NOT NULL CHECK (runtime_ms >= 0),
    CONSTRAINT agent_runtime_hourly_usage_hour_aligned CHECK (date_trunc('hour', timezone('UTC', bucket_start)) = timezone('UTC', bucket_start)),
    PRIMARY KEY (bucket_start, organization_id, group_id, user_id)
);

COMMENT ON TABLE agent_runtime_hourly_usage IS 'Agent Runtime per hour, organization, effective Agent Hours group, and chat owner. Written with each hb_agent_runtime_v1 usage event and never purged. No foreign keys, so totals survive deleted users, groups, and organizations.';

-- Per-group totals of one organization.
CREATE INDEX idx_agent_runtime_hourly_usage_org ON agent_runtime_hourly_usage (organization_id, bucket_start) INCLUDE (group_id, runtime_ms);
-- Member totals within one group.
CREATE INDEX idx_agent_runtime_hourly_usage_group_user ON agent_runtime_hourly_usage (group_id, user_id, bucket_start) INCLUDE (runtime_ms);

-- agent_hours_effective_group_id returns the group a user's Agent Runtime in
-- an organization counts toward: the user's group in that organization with
-- the largest Agent Hours allotment, ties broken by the lowest group ID, or
-- the organization's Everyone group when no allotted group contains the user.
-- Everyone is the fallback, never a candidate, even if it has an allotment
-- row.
CREATE FUNCTION agent_hours_effective_group_id(arg_organization_id UUID, arg_user_id UUID) RETURNS UUID
    LANGUAGE sql STABLE
    AS $$
    SELECT COALESCE((
        SELECT a.group_id
        FROM agent_hours_group_allotments a
        JOIN groups g ON g.id = a.group_id
        JOIN group_members gm ON gm.group_id = a.group_id
        WHERE g.organization_id = arg_organization_id
          AND gm.user_id = arg_user_id
          AND a.group_id <> arg_organization_id
        ORDER BY a.allotment_bps DESC, a.group_id ASC
        LIMIT 1
    ), arg_organization_id)
$$;

-- Backfill the hours that already have a usage event from the chat messages
-- that still exist. Group allotments are new, so every owner falls back to
-- the Everyone group. Hours whose chats were purged stay unattributed.
INSERT INTO agent_runtime_hourly_usage (bucket_start, organization_id, group_id, user_id, runtime_ms)
SELECT
    hour_event.created_at,
    c.organization_id,
    c.organization_id,
    c.owner_id,
    SUM(cm.runtime_ms)
FROM usage_events hour_event
JOIN chat_messages cm
    ON cm.created_at >= hour_event.created_at
    AND cm.created_at < hour_event.created_at + INTERVAL '1 hour'
    AND cm.runtime_ms IS NOT NULL
JOIN chats c ON c.id = cm.chat_id
WHERE hour_event.event_type = 'hb_agent_runtime_v1'
GROUP BY hour_event.created_at, c.organization_id, c.owner_id
HAVING SUM(cm.runtime_ms) > 0;
