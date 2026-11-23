CREATE TYPE chat_project_pin_source AS ENUM (
    'migration',
    'default',
    'user'
);

CREATE TABLE chat_project_pins (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id uuid NOT NULL REFERENCES chat_projects(id) ON DELETE CASCADE,
    pin_order integer NOT NULL CONSTRAINT chat_project_pins_pin_order_check CHECK (pin_order > 0),
    source chat_project_pin_source NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, project_id)
);

COMMENT ON TABLE chat_project_pins IS 'Per-user pinned chat projects. Pins can outlive project access, so readers must check access.';

CREATE INDEX idx_chat_project_pins_user_order ON chat_project_pins (user_id, pin_order);

-- Pin every project its owner can already see in the sidebar, in the
-- sidebar's order, so nothing disappears on upgrade. Shared projects are
-- not pinned.
INSERT INTO chat_project_pins (user_id, project_id, pin_order, source)
SELECT
    owner_id,
    id,
    ROW_NUMBER() OVER (PARTITION BY owner_id ORDER BY lower(name), id),
    'migration'
FROM chat_projects
ON CONFLICT DO NOTHING;

-- Projects listed for a user: the ones they own and, when include_grants is
-- true, the ones whose user or group ACL names them or a group they belong
-- to (including the Everyone group) in an organization they belong to.
-- Grants match whatever actions they hold, so callers must still authorize
-- each project. The planner inlines this function, so the ACL indexes still
-- apply.
CREATE FUNCTION chat_project_ids_for_user(viewer_id uuid, include_grants boolean)
RETURNS TABLE (project_id uuid)
LANGUAGE sql STABLE
AS $$
SELECT chat_projects.id
FROM chat_projects
WHERE chat_projects.owner_id = viewer_id
    OR (
        include_grants
        AND (
            chat_projects.user_acl ? viewer_id::text
            OR chat_projects.group_acl ?| ARRAY(
                SELECT group_members.group_id::text
                FROM group_members
                WHERE group_members.user_id = viewer_id
                UNION ALL
                SELECT organization_members.organization_id::text
                FROM organization_members
                WHERE organization_members.user_id = viewer_id
            )
        )
        AND EXISTS (
            SELECT 1
            FROM organization_members
            WHERE organization_members.user_id = viewer_id
                AND organization_members.organization_id = chat_projects.organization_id
        )
    )
$$;

-- Users that read grants on chat projects reach: granted users, and members
-- of granted groups including the Everyone group, who belong to the
-- project's organization. Only grants that include read count, as in
-- IsChatProjectUsableByUserID. The owner is not included, and a user
-- reached by several grants appears once per grant.
CREATE VIEW chat_project_read_grant_members AS
SELECT chat_projects.id AS project_id, organization_members.user_id
FROM chat_projects
CROSS JOIN LATERAL jsonb_each(chat_projects.user_acl) AS grants(principal, entry)
JOIN organization_members
    ON organization_members.user_id = grants.principal::uuid
    AND organization_members.organization_id = chat_projects.organization_id
WHERE grants.entry -> 'permissions' ?| ARRAY['read', '*']
UNION ALL
SELECT chat_projects.id AS project_id, group_members_expanded.user_id
FROM chat_projects
CROSS JOIN LATERAL jsonb_each(chat_projects.group_acl) AS grants(principal, entry)
JOIN group_members_expanded ON group_members_expanded.group_id = grants.principal::uuid
-- A granted group from another organization only reaches its members who
-- also belong to the project's organization.
JOIN organization_members
    ON organization_members.user_id = group_members_expanded.user_id
    AND organization_members.organization_id = chat_projects.organization_id
WHERE grants.entry -> 'permissions' ?| ARRAY['read', '*'];
