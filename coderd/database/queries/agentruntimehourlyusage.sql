-- name: GetAgentRuntimeHourlyUsage :many
-- Computes one bucket of agent_runtime_hourly_usage from chat messages: the
-- runtime per chat organization and owner, with the owner's current
-- effective Agent Hours group. Like the hb_agent_runtime_v1 payload, it
-- counts soft-deleted messages and messages from all chats.
WITH chat_runtime AS (
    SELECT
        cm.chat_id,
        SUM(cm.runtime_ms)::bigint AS runtime_ms
    FROM chat_messages cm
    WHERE cm.created_at >= @start_time::timestamptz
      AND cm.created_at < @end_time::timestamptz
      AND cm.runtime_ms IS NOT NULL
    GROUP BY cm.chat_id
)
SELECT
    c.organization_id,
    agent_hours_effective_group_id(c.organization_id, c.owner_id)::uuid AS group_id,
    c.owner_id AS user_id,
    SUM(cr.runtime_ms)::bigint AS runtime_ms
FROM chat_runtime cr
JOIN chats c ON c.id = cr.chat_id
GROUP BY c.organization_id, c.owner_id
-- Owners whose runtime sums to zero add nothing to the bucket.
HAVING SUM(cr.runtime_ms) <> 0;

-- name: InsertAgentRuntimeHourlyUsage :exec
INSERT INTO agent_runtime_hourly_usage (
    bucket_start,
    organization_id,
    group_id,
    user_id,
    runtime_ms
)
SELECT
    @bucket_start::timestamptz,
    unnest(@organization_ids::uuid[]),
    unnest(@group_ids::uuid[]),
    unnest(@user_ids::uuid[]),
    unnest(@runtime_ms::bigint[]);

-- name: GetAgentRuntimeUsageByOrganization :many
-- Sums agent_runtime_hourly_usage per organization over the buckets that
-- start in [start_time, end_time), which is how the license total counts
-- buckets. Deleted organizations, which are soft-deleted, have empty names.
WITH usage_by_organization AS (
    SELECT
        organization_id,
        SUM(runtime_ms)::bigint AS runtime_ms
    FROM agent_runtime_hourly_usage
    WHERE bucket_start >= @start_time::timestamptz
      AND bucket_start < @end_time::timestamptz
    GROUP BY organization_id
)
SELECT
    usage_by_organization.organization_id,
    COALESCE(organizations.name, '')::text AS organization_name,
    COALESCE(organizations.display_name, '')::text AS organization_display_name,
    usage_by_organization.runtime_ms
FROM usage_by_organization
LEFT JOIN organizations
    ON organizations.id = usage_by_organization.organization_id
    AND NOT organizations.deleted
ORDER BY usage_by_organization.organization_id;

-- name: GetAgentRuntimeUsageByGroup :many
-- Sums one organization's agent_runtime_hourly_usage per effective group
-- over the buckets that start in [start_time, end_time). The Everyone
-- group's ID is the organization ID. Deleted groups have empty names.
WITH usage_by_group AS (
    SELECT
        group_id,
        SUM(runtime_ms)::bigint AS runtime_ms
    FROM agent_runtime_hourly_usage
    WHERE agent_runtime_hourly_usage.organization_id = @organization_id
      AND bucket_start >= @start_time::timestamptz
      AND bucket_start < @end_time::timestamptz
    GROUP BY group_id
)
SELECT
    usage_by_group.group_id,
    COALESCE(groups.name, '')::text AS group_name,
    COALESCE(groups.display_name, '')::text AS group_display_name,
    usage_by_group.runtime_ms
FROM usage_by_group
LEFT JOIN groups ON groups.id = usage_by_group.group_id
ORDER BY usage_by_group.group_id;

-- name: GetGroupMembersAgentRuntimeUsage :many
-- Returns each requested user who is a member of the group, with the
-- runtime attributed to the group over the buckets that start in
-- [start_time, end_time) and the user's current effective Agent Hours group
-- in the group's organization. Uses group_members_expanded so the implicit
-- Everyone group counts.
WITH members AS (
    SELECT DISTINCT
        user_id,
        organization_id,
        agent_hours_effective_group_id(organization_id, user_id)::uuid AS effective_group_id
    FROM group_members_expanded
    WHERE group_members_expanded.group_id = @group_id
      AND group_members_expanded.user_id = ANY(@user_ids::uuid[])
)
SELECT
    members.user_id,
    members.organization_id,
    COALESCE((
        SELECT SUM(hourly.runtime_ms)
        FROM agent_runtime_hourly_usage hourly
        WHERE hourly.group_id = @group_id
          AND hourly.user_id = members.user_id
          AND hourly.bucket_start >= @start_time::timestamptz
          AND hourly.bucket_start < @end_time::timestamptz
    ), 0)::bigint AS runtime_ms,
    members.effective_group_id,
    COALESCE(effective.name, '')::text AS effective_group_name,
    COALESCE(effective.display_name, '')::text AS effective_group_display_name
FROM members
LEFT JOIN groups effective ON effective.id = members.effective_group_id
ORDER BY members.user_id;
